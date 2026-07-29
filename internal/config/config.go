// Package config locates, loads, and persists revision's user configuration.
// It is a self-contained infrastructure layer, deliberately domain-agnostic: it
// knows nothing about the TUI, the SVN domain, or the app composition, so any
// layer can read or update settings without creating an import cycle.
//
// The configuration lives in a single JSON document at
// $XDG_CONFIG_HOME/revision/config.json — which is ~/.config/revision/config.json
// on both macOS and Linux when XDG_CONFIG_HOME is unset. Go's os.UserConfigDir
// is intentionally NOT used: it resolves to ~/Library/Application Support on
// macOS, whereas revision keeps a single ~/.config location across both
// platforms.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
)

const (
	// appDir is the per-user configuration subdirectory under the config home.
	appDir = "revision"
	// fileName is the configuration document revision reads and writes.
	fileName = "config.json"

	// dirPerm and filePerm are the permissions used when creating the config
	// directory and file. The config may reference local paths but holds no
	// secrets, so standard user-owned rwx/rw perms are appropriate.
	dirPerm  fs.FileMode = 0o755
	filePerm fs.FileMode = 0o644
)

// The display scopes accepted by Config.DisplayFrom name the directory revision
// shows the working copy from.
const (
	// DisplayFromCWD limits the views to the directory revision was started in.
	DisplayFromCWD = "cwd"
	// DisplayFromRoot widens the views to the working copy (sandbox) root, no
	// matter which directory inside it revision was started in.
	DisplayFromRoot = "root"
)

// DisplayFromValues lists the supported display scopes, in the order a chooser
// should offer them.
func DisplayFromValues() []string { return []string{DisplayFromCWD, DisplayFromRoot} }

// Config holds revision's persisted user settings. Every field maps to a JSON
// key; unknown keys in an on-disk file are ignored, and any key omitted from
// the file falls back to its Default value when loaded.
type Config struct {
	// LogLimit caps how many revisions the Log panel requests. It must be
	// positive; non-positive values found on disk are normalized back to the
	// default when loaded.
	LogLimit int `json:"logLimit"`
	// Editor is the external editor invoked for commit messages. Empty means
	// use the in-app editor.
	Editor string `json:"editor"`
	// Theme selects the color palette by name. Empty is normalized to the
	// built-in default palette.
	Theme string `json:"theme"`
	// DirectoryDiff controls whether highlighting a directory row shows the
	// combined diff of every change beneath it. When false, the directory diff
	// is hidden by default and can be toggled on at runtime.
	DirectoryDiff bool `json:"directoryDiff"`
	// HideUntracked controls whether untracked (unversioned) files are omitted
	// from the Changes and diff views. When false (the default) untracked files
	// are shown; when true they are hidden globally and can be revealed on demand
	// with a runtime toggle.
	HideUntracked bool `json:"hideUntracked"`
	// SSHKeyPath is the SSH private key used to authenticate against a remote
	// repository over svn+ssh. A blank value is normalized to the default key
	// location, ~/.ssh/id_rsa, so the setting always names a concrete key. A
	// leading ~ is expanded to the user's home directory when the key is used.
	SSHKeyPath string `json:"sshKeyPath"`
	// DisplayFrom selects the directory the working copy is displayed from:
	// DisplayFromCWD (the default) shows only what lies under the directory
	// revision was started in, while DisplayFromRoot shows the whole working copy
	// from its root. Any other value is reset to the default when loaded.
	DisplayFrom string `json:"displayFrom"`
}

// Default returns the configuration used when no file exists yet or when a
// field is absent from the on-disk document.
func Default() Config {
	return Config{
		LogLimit:      100,
		Editor:        "",
		Theme:         "auto",
		DirectoryDiff: true,
		HideUntracked: false,
		SSHKeyPath:    "~/.ssh/id_rsa",
		DisplayFrom:   DisplayFromCWD,
	}
}

// validDisplayFrom reports whether v names a display scope the schema defines,
// ignoring surrounding whitespace.
func validDisplayFrom(v string) bool {
	switch strings.TrimSpace(v) {
	case DisplayFromCWD, DisplayFromRoot:
		return true
	}
	return false
}

// Validator inspects a loaded Config for values that parse correctly but are no
// longer supported by the running build — for example, a theme removed in an
// update — resetting each to its default so the new default takes precedence. It
// returns a human-readable description of every value it resets and must leave
// valid values untouched. Reconcile treats a nil Validator as "no domain checks".
type Validator func(cfg *Config) []string

// Reconciliation reports how Reconcile brought the on-disk config up to date so
// the caller can inform the user. Conflicts holds one human-readable description
// per value that was invalid under the current schema and reset to its default.
type Reconciliation struct {
	// Created is true when no file existed and one was written with defaults.
	Created bool
	// Updated is true when an existing file was rewritten, whether to merge in
	// keys a newer build added or to reset conflicting values.
	Updated bool
	// Conflicts lists, in order, every value that was invalid under the current
	// schema and reset to its default. It is empty when nothing conflicted.
	Conflicts []string
}

// Notice returns a short, user-facing summary of any conflicts resolved during
// reconciliation, or the empty string when there is nothing to report. Merging
// in keys a newer build added is silent; only values reset to their defaults are
// surfaced, since those discard a setting the user may have chosen.
func (r Reconciliation) Notice() string {
	switch len(r.Conflicts) {
	case 0:
		return ""
	case 1:
		return "config: " + r.Conflicts[0]
	default:
		return fmt.Sprintf("config: %d settings reset to defaults", len(r.Conflicts))
	}
}

// Dir returns the directory that holds revision's configuration:
// $XDG_CONFIG_HOME/revision when XDG_CONFIG_HOME is set, otherwise
// ~/.config/revision. It does not create the directory.
func Dir() (string, error) {
	if xdg := strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME")); xdg != "" {
		return filepath.Join(xdg, appDir), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("locate home directory: %w", err)
	}
	return filepath.Join(home, ".config", appDir), nil
}

// Path returns the absolute path to revision's configuration file,
// ~/.config/revision/config.json by default. It does not create the file.
func Path() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, fileName), nil
}

// Load reads and parses the configuration file, filling any absent field from
// Default. A missing file is not an error: first-run callers get Default so the
// app works before a config file exists. Unreadable files and malformed JSON
// are returned as errors, along with Default so the caller can still proceed.
func Load() (Config, error) {
	path, err := Path()
	if err != nil {
		return Default(), err
	}
	return loadFrom(path)
}

// loadFrom is the path-explicit core of Load, kept separate so tests can read
// from a temporary location without touching the real home directory.
func loadFrom(path string) (Config, error) {
	// Decoding onto the defaults means keys omitted from the file keep their
	// default values rather than becoming Go zero values.
	cfg := Default()

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return cfg, nil
		}
		return Default(), fmt.Errorf("read config %s: %w", path, err)
	}

	if err := json.Unmarshal(data, &cfg); err != nil {
		return Default(), fmt.Errorf("parse config %s: %w", path, err)
	}

	cfg.normalize()
	return cfg, nil
}

// Reconcile loads the configuration and brings the on-disk file in line with the
// current schema, so upgrading the application never leaves a stale config.json
// behind. It is the startup entry point. When a newer build has introduced
// settings the file predates, their defaults are merged in and the file is
// rewritten. When a stored value conflicts with the current schema — it is
// invalid, or (via validate) no longer supported — it is reset to its default,
// the new default taking precedence, and reported on the returned Reconciliation
// so the caller can surface a message. A missing file is created with defaults,
// exactly like a first run. validate may be nil to skip domain-specific checks.
// A failed write returns the reconciled config alongside the error so the app
// can still start.
func Reconcile(validate Validator) (Config, Reconciliation, error) {
	path, err := Path()
	if err != nil {
		return Default(), Reconciliation{}, err
	}
	return reconcileAt(path, validate)
}

// reconcileAt is the path-explicit core of Reconcile, kept separate so tests can
// work in a temporary location without touching the real home directory.
func reconcileAt(path string, validate Validator) (Config, Reconciliation, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			cfg := Default()
			if err := saveTo(path, cfg); err != nil {
				return cfg, Reconciliation{}, err
			}
			return cfg, Reconciliation{Created: true}, nil
		}
		return Default(), Reconciliation{}, fmt.Errorf("read config %s: %w", path, err)
	}

	// present records which keys the document actually contains, so a key set to
	// its zero value can be told apart from one omitted entirely.
	var present map[string]json.RawMessage
	if err := json.Unmarshal(data, &present); err != nil {
		return Default(), Reconciliation{}, fmt.Errorf("parse config %s: %w", path, err)
	}

	// Decoding onto the defaults means keys omitted from the file keep their
	// default values rather than becoming Go zero values.
	cfg := Default()
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Default(), Reconciliation{}, fmt.Errorf("parse config %s: %w", path, err)
	}

	rec := Reconciliation{Conflicts: cfg.reconcileValues(present, validate)}

	// Rewrite the file when a newer build added keys it lacks or when a value had
	// to be reset, so the persisted document always matches the running schema.
	if fileMissingKeys(present) || len(rec.Conflicts) > 0 {
		rec.Updated = true
		if err := saveTo(path, cfg); err != nil {
			return cfg, rec, err
		}
	}
	return cfg, rec, nil
}

// normalize repairs values that are invalid on disk so callers always receive a
// usable Config. It runs only on externally sourced input (a loaded file).
func (c *Config) normalize() {
	def := Default()
	if c.LogLimit <= 0 {
		c.LogLimit = def.LogLimit
	}
	if strings.TrimSpace(c.Theme) == "" {
		c.Theme = def.Theme
	}
	if strings.TrimSpace(c.SSHKeyPath) == "" {
		c.SSHKeyPath = def.SSHKeyPath
	}
	c.DisplayFrom = strings.TrimSpace(c.DisplayFrom)
	if !validDisplayFrom(c.DisplayFrom) {
		c.DisplayFrom = def.DisplayFrom
	}
}

// reconcileValues brings the loaded settings in line with the current schema. It
// applies the same silent repairs as normalize (a non-positive logLimit, a blank
// theme) and, through the optional validate hook, any domain-level ones such as a
// theme removed in an update. Every value it resets to a default is returned as a
// human-readable conflict so the caller can tell the user. Only keys actually
// present on disk can conflict: an absent key already holds its default, and a
// blank value is treated as a silent omission, so neither is reported.
func (c *Config) reconcileValues(present map[string]json.RawMessage, validate Validator) []string {
	def := Default()
	var conflicts []string

	// Record the original logLimit before normalize repairs it, so the message
	// can name the offending value. Only an explicitly written value conflicts.
	if _, ok := present["logLimit"]; ok && c.LogLimit <= 0 {
		conflicts = append(conflicts,
			fmt.Sprintf("logLimit %d is invalid; reset to %d", c.LogLimit, def.LogLimit))
	}

	// A displayFrom naming a scope the schema doesn't define cannot be honored, so
	// report which value was dropped before normalize replaces it.
	if _, ok := present["displayFrom"]; ok && strings.TrimSpace(c.DisplayFrom) != "" && !validDisplayFrom(c.DisplayFrom) {
		conflicts = append(conflicts,
			fmt.Sprintf("displayFrom %q is invalid; reset to %q", c.DisplayFrom, def.DisplayFrom))
	}

	c.normalize()

	if validate != nil {
		conflicts = append(conflicts, validate(c)...)
	}
	return conflicts
}

// schemaKeys returns the JSON object keys the current Config schema defines,
// derived from the struct tags so fields added in a later build are picked up
// automatically without maintaining a second list.
func schemaKeys() []string {
	t := reflect.TypeOf(Config{})
	keys := make([]string, 0, t.NumField())
	for i := 0; i < t.NumField(); i++ {
		name := strings.SplitN(t.Field(i).Tag.Get("json"), ",", 2)[0]
		if name == "" || name == "-" {
			continue
		}
		keys = append(keys, name)
	}
	return keys
}

// fileMissingKeys reports whether the on-disk document omits any key the current
// schema defines. A missing key means a newer build introduced a setting the
// file predates, so the file must be rewritten to include its default.
func fileMissingKeys(present map[string]json.RawMessage) bool {
	for _, key := range schemaKeys() {
		if _, ok := present[key]; !ok {
			return true
		}
	}
	return false
}

// Save writes cfg to the configuration file as indented JSON, creating the
// ~/.config/revision directory if necessary.
func Save(cfg Config) error {
	path, err := Path()
	if err != nil {
		return err
	}
	return saveTo(path, cfg)
}

// saveTo is the path-explicit core of Save. The write is atomic: the document is
// written to a temporary file in the same directory and renamed into place, so
// an interrupted write can never leave a truncated config behind.
func saveTo(path string, cfg Config) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, dirPerm); err != nil {
		return fmt.Errorf("create config dir %s: %w", dir, err)
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("encode config: %w", err)
	}
	data = append(data, '\n')

	tmp, err := os.CreateTemp(dir, fileName+".*.tmp")
	if err != nil {
		return fmt.Errorf("create temp config: %w", err)
	}
	tmpName := tmp.Name()
	// Best-effort cleanup if we return before the rename succeeds; after a
	// successful rename the temp file no longer exists and Remove is a no-op.
	defer func() { _ = os.Remove(tmpName) }()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write temp config: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp config: %w", err)
	}
	if err := os.Chmod(tmpName, filePerm); err != nil {
		return fmt.Errorf("chmod temp config: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("replace config %s: %w", path, err)
	}
	return nil
}
