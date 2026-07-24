// Command revision is a lazygit-style terminal UI for Subversion.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/bapatchirag/revision/internal/app"
	"github.com/bapatchirag/revision/internal/svn"
	tea "github.com/charmbracelet/bubbletea"
)

// version is overridden via -ldflags at build time.
var version = "dev"

func main() {
	var (
		path        string
		showVersion bool
	)
	flag.StringVar(&path, "path", ".", "path to the SVN working copy to operate on")
	flag.BoolVar(&showVersion, "version", false, "print version and exit")
	flag.Usage = usage
	flag.Parse()

	if showVersion {
		_, _ = fmt.Printf("revision %s\n", version)
		return
	}

	if err := run(path); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "revision:", err)
		os.Exit(1)
	}
}

func usage() {
	_, _ = fmt.Fprintf(os.Stderr, "revision %s — a lazygit-style TUI for Subversion\n\n"+
		"Usage:\n  revision [flags]\n\nFlags:\n", version)
	flag.PrintDefaults()
}

func run(path string) error {
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

	program := tea.NewProgram(app.New(client, info), tea.WithAltScreen())
	_, err = program.Run()
	return err
}
