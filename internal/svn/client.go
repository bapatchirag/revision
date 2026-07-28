package svn

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// DefaultBinary is the svn executable used when a Client does not override it.
const DefaultBinary = "svn"

// CommandRecord captures a single svn invocation and its result so the UI can
// mirror the actual commands revision runs. Err is empty on success.
type CommandRecord struct {
	// Command is the full command line, e.g. "svn status --xml --non-interactive".
	Command string
	// Subcommand is the svn subcommand (args[0]), e.g. "commit" or "status", so
	// callers can classify an invocation without parsing Command.
	Subcommand string
	// Output is the command's trimmed stdout.
	Output string
	// Err is the trimmed stderr (or error text) when the command failed; empty
	// on success.
	Err string
	// Duration is how long the command took to run.
	Duration time.Duration
}

// Client runs svn commands against a working-copy directory.
type Client struct {
	// Dir is the working directory svn commands run in (the working copy).
	Dir string
	// Bin is the svn executable name or path. Empty means DefaultBinary.
	Bin string
	// Recorder, when set, receives one CommandRecord per svn invocation as it
	// completes. It may be called from multiple goroutines concurrently, so an
	// implementation must be safe for concurrent use.
	Recorder func(CommandRecord)
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
// When a Recorder is set it receives one CommandRecord per invocation.
func (c *Client) run(ctx context.Context, args ...string) ([]byte, error) {
	full := append(append([]string{}, args...), "--non-interactive")
	cmd := exec.CommandContext(ctx, c.binary(), full...)
	cmd.Dir = c.Dir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	start := time.Now()
	runErr := cmd.Run()
	elapsed := time.Since(start)

	if c.Recorder != nil {
		rec := CommandRecord{
			Command:  c.binary() + " " + strings.Join(full, " "),
			Output:   strings.TrimRight(stdout.String(), "\n"),
			Duration: elapsed,
		}
		if len(args) > 0 {
			rec.Subcommand = args[0]
		}
		if runErr != nil {
			errText := strings.TrimSpace(stderr.String())
			if errText == "" {
				errText = runErr.Error()
			}
			rec.Err = errText
		}
		c.Recorder(rec)
	}

	if runErr != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = runErr.Error()
		}
		return nil, fmt.Errorf("svn %s: %s", strings.Join(args, " "), msg)
	}
	return stdout.Bytes(), nil
}
