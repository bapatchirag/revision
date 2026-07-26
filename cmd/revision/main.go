// Command revision is a lazygit-style terminal UI for Subversion.
package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/bapatchirag/revision/internal/app"
	"github.com/bapatchirag/revision/internal/config"
	"github.com/bapatchirag/revision/internal/selfupdate"
	"github.com/bapatchirag/revision/internal/svn"
	tea "github.com/charmbracelet/bubbletea"
)

// version and channel are overridden via -ldflags at build time. channel is
// "release" only for official release builds; every development or locally
// cross-compiled build keeps the default, which disables the self-update paths.
var (
	version = "dev"
	channel = "dev"
)

func main() {
	var (
		path        string
		showVersion bool
		doUpdate    bool
		updateWith  string
	)
	flag.StringVar(&path, "path", ".", "path to the SVN working copy to operate on")
	flag.BoolVar(&showVersion, "version", false, "print version and exit")
	flag.BoolVar(&doUpdate, "update", false, "check for a newer release and update the binary")
	flag.StringVar(&updateWith, "update-with", "", "update method for --update: 'curl' or 'go' (default: prompt)")
	flag.Usage = usage
	flag.Parse()

	build := selfupdate.Build{Version: version, Channel: channel}

	if showVersion {
		_, _ = fmt.Printf("revision %s\n", version)
		return
	}

	if doUpdate {
		if err := runUpdate(build, updateWith); err != nil {
			_, _ = fmt.Fprintln(os.Stderr, "revision:", err)
			os.Exit(1)
		}
		return
	}

	if err := run(path, build); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "revision:", err)
		os.Exit(1)
	}
}

func usage() {
	_, _ = fmt.Fprintf(os.Stderr, "revision %s — a lazygit-style TUI for Subversion\n\n"+
		"Usage:\n  revision [flags]\n\nFlags:\n", version)
	flag.PrintDefaults()
}

func run(path string, build selfupdate.Build) error {
	if _, err := exec.LookPath("svn"); err != nil {
		return fmt.Errorf("the 'svn' command was not found on your PATH; please install Subversion")
	}

	abs, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("resolve path %q: %w", path, err)
	}

	client := svn.New(abs)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	info, err := client.Info(ctx)
	if err != nil {
		if svn.IsAuthError(err) {
			return fmt.Errorf("cannot read the working copy at %q: %s", abs, svn.AuthHint)
		}
		return fmt.Errorf("%q does not appear to be an SVN working copy: %w", abs, err)
	}

	cfg, err := config.Ensure()
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "revision: using default config:", err)
	}

	model := app.New(client, info, build, cfg)
	program := tea.NewProgram(model, tea.WithAltScreen())
	final, err := program.Run()
	if err != nil {
		return err
	}

	// If the user chose to self-update from the startup prompt, apply it now
	// that the alt-screen is gone and the terminal is back to normal.
	if m, ok := final.(*app.Model); ok {
		if method, chosen := m.PendingUpdate(); chosen {
			return applyUpdate(method)
		}
	}
	return nil
}

// runUpdate implements the `--update` CLI path: it refuses to run on a
// development build, checks GitHub for a newer release, and — when one exists —
// applies it using the requested method (or an interactive choice).
func runUpdate(build selfupdate.Build, method string) error {
	if !build.IsRelease() {
		_, _ = fmt.Printf("revision %s is a development build; self-update is only available for release builds.\n", version)
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	rel, newer, err := selfupdate.Check(ctx, build)
	if err != nil {
		return fmt.Errorf("checking for updates: %w", err)
	}
	if !newer {
		_, _ = fmt.Printf("revision %s is already up to date.\n", version)
		return nil
	}

	_, _ = fmt.Printf("A new version is available: %s (current %s)\n", rel.Tag, version)
	m, ok, err := resolveMethod(method)
	if err != nil {
		return err
	}
	if !ok {
		_, _ = fmt.Println("Update cancelled.")
		return nil
	}
	return applyUpdate(m)
}

// resolveMethod turns the --update-with flag into a method. An explicit "curl"
// or "go" is used directly; an empty value falls back to an interactive prompt
// that mirrors the in-app choices; anything else is rejected.
func resolveMethod(method string) (selfupdate.Method, bool, error) {
	switch strings.ToLower(strings.TrimSpace(method)) {
	case "curl":
		return selfupdate.MethodCurl, true, nil
	case "go":
		return selfupdate.MethodGo, true, nil
	case "":
		m, ok := promptMethod()
		return m, ok, nil
	default:
		return 0, false, fmt.Errorf("unknown --update-with %q (want 'curl' or 'go')", method)
	}
}

// promptMethod asks the user how to update, mirroring the in-app prompt. A
// non-interactive stdin (EOF) cancels the update.
func promptMethod() (selfupdate.Method, bool) {
	_, _ = fmt.Println("How would you like to update?")
	_, _ = fmt.Println("  1) Update with cURL")
	_, _ = fmt.Println("  2) Update with Go")
	_, _ = fmt.Println("  3) Don't update this time")
	_, _ = fmt.Print("Select [1-3]: ")

	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil && line == "" {
		return 0, false
	}
	switch strings.TrimSpace(line) {
	case "1":
		return selfupdate.MethodCurl, true
	case "2":
		return selfupdate.MethodGo, true
	default:
		return 0, false
	}
}

// applyUpdate runs the chosen update method and prints a follow-up hint on
// success so the user knows to restart.
func applyUpdate(method selfupdate.Method) error {
	_, _ = fmt.Printf("Updating revision with %s…\n", method.Label())
	if err := selfupdate.Run(method); err != nil {
		return fmt.Errorf("update failed: %w", err)
	}
	_, _ = fmt.Println("Update complete. Re-run 'revision' to use the new version.")
	return nil
}
