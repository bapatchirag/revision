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

// Config holds revision's persisted user settings. Every field maps to a JSON
// key; unknown keys in an on-disk file are ignored, and any key omitted from
// the file falls back to its Default value when loaded.
type Config struct {
	// DefaultPath is the working copy revision opens when no -path flag is
	// given. Empty means the current directory.
	DefaultPath string `json:"defaultPath"`
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
}

// Default returns the configuration used when no file exists yet or when a
// field is absent from the on-disk document.
func Default() Config {
	return Config{
		DefaultPath:   "",
		LogLimit:      100,
		Editor:        "",
		Theme:         "auto",
		DirectoryDiff: true,
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

// Ensure loads the configuration, creating the file with default values when it
// does not yet exist. It is intended for startup: the first run persists a
// config.json populated with defaults, giving users a documented file to edit,
// while later runs behave exactly like Load. Only an absent file triggers a
// write; every other case defers to Load, so normalization and error handling
// are unchanged. A failed write returns Default alongside the error so the app
// can still start.
func Ensure() (Config, error) {
	path, err := Path()
	if err != nil {
		return Default(), err
	}

	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			cfg := Default()
			if err := saveTo(path, cfg); err != nil {
				return cfg, err
			}
			return cfg, nil
		}
		return Default(), fmt.Errorf("stat config %s: %w", path, err)
	}

	return loadFrom(path)
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
