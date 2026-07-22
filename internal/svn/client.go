package svn

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// DefaultBinary is the svn executable used when a Client does not override it.
const DefaultBinary = "svn"

// Client runs svn commands against a working-copy directory.
type Client struct {
	// Dir is the working directory svn commands run in (the working copy).
	Dir string
	// Bin is the svn executable name or path. Empty means DefaultBinary.
	Bin string
}

// New returns a Client operating on the given working-copy directory.
func New(dir string) *Client {
	return &Client{Dir: dir, Bin: DefaultBinary}
}

// binary returns the svn executable to invoke.
func (c *Client) binary() string {
	if c.Bin == "" {
		return DefaultBinary
	}
	return c.Bin
}

// run executes `svn <args...> --non-interactive` in the client's directory and
// returns stdout. On failure it returns an error that includes svn's stderr.
// --non-interactive is always appended so svn never blocks on a credential prompt.
func (c *Client) run(ctx context.Context, args ...string) ([]byte, error) {
	full := append(append([]string{}, args...), "--non-interactive")
	cmd := exec.CommandContext(ctx, c.binary(), full...)
	cmd.Dir = c.Dir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("svn %s: %s", strings.Join(args, " "), msg)
	}
	return stdout.Bytes(), nil
}
