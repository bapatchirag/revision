package svn

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
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
	// UserAction reports that the invocation ran under a context marked by
	// WithUserAction: something the user asked for, rather than a query the
	// caller runs on its own.
	UserAction bool
	// Duration is how long the command took to run.
	Duration time.Duration
}

// userActionKey marks a context as carrying a user-requested command.
type userActionKey struct{}

// WithUserAction marks every svn command run under the returned context as one
// the user asked for. It lets a caller tell its own background queries apart
// from the same subcommand run on demand — a diff loaded to fill a panel from
// one the user asked to be written out.
func WithUserAction(ctx context.Context) context.Context {
	return context.WithValue(ctx, userActionKey{}, true)
}

func isUserAction(ctx context.Context) bool {
	v, _ := ctx.Value(userActionKey{}).(bool)
	return v
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
// returns stdout. On failure it returns an error that includes svn's stderr, or
// the deadline it overran when ctx timed out (see failureText).
// --non-interactive is always appended so svn never blocks on a credential prompt.
// The command runs under LC_ALL=C, since revision reads svn's own words back —
// "Reverted", "Skipped" — and svn translates them under any other locale.
// When a Recorder is set it receives one CommandRecord per invocation.
func (c *Client) run(ctx context.Context, args ...string) ([]byte, error) {
	full := append(append([]string{}, args...), "--non-interactive")
	cmd := exec.CommandContext(ctx, c.binary(), full...)
	cmd.Dir = c.Dir
	cmd.Env = append(os.Environ(), "LC_ALL=C")
	boundCancellation(cmd)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	start := time.Now()
	runErr := cmd.Run()
	elapsed := time.Since(start)
	if errors.Is(runErr, exec.ErrWaitDelay) {
		// exec only reports this for a command that exited successfully: svn is
		// done and its output is complete, and all that overran is a child holding
		// the pipe open behind it (an ssh ControlMaster, say). Not a failure.
		runErr = nil
	}

	var msg string
	if runErr != nil {
		msg = failureText(ctx, stderr.String(), runErr, elapsed)
	}

	if c.Recorder != nil {
		rec := CommandRecord{
			Command:    c.binary() + " " + strings.Join(full, " "),
			Output:     strings.TrimRight(stdout.String(), "\n"),
			UserAction: isUserAction(ctx),
			Duration:   elapsed,
			Err:        msg,
		}
		if len(args) > 0 {
			rec.Subcommand = args[0]
		}
		c.Recorder(rec)
	}

	if runErr != nil {
		return nil, fmt.Errorf("svn %s: %s", strings.Join(args, " "), msg)
	}
	return stdout.Bytes(), nil
}

// failureText explains why a command failed. A deadline that expired is reported
// as the timeout it was: exec kills the process with SIGKILL, so svn dies before
// it can say anything and runErr is a bare "signal: killed" that names the signal
// rather than the cause. Otherwise svn's own stderr stands, falling back to the
// exec error when it died without a word.
func failureText(ctx context.Context, stderr string, runErr error, elapsed time.Duration) string {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return fmt.Sprintf("timed out after %s", elapsed.Round(time.Second))
	}
	if msg := strings.TrimSpace(stderr); msg != "" {
		return msg
	}
	return runErr.Error()
}

// runGrace bounds how long a killed command's output pipes are waited on before
// they are closed from under whatever still holds them.
const runGrace = 2 * time.Second

// boundCancellation makes a timed-out or abandoned command actually end. exec
// kills only the process it started, and Wait then blocks until every writer to
// the output pipes is gone — so the ssh that svn+ssh forks, still stuck on the
// network, keeps run from ever returning and the reply never reaches the caller.
// A refresh cannot rescue that: it only starts a second read that hangs the same
// way. The command therefore gets its own process group, cancellation kills the
// whole group, and WaitDelay closes the pipes as a backstop.
func boundCancellation(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); err != nil {
			if errors.Is(err, syscall.ESRCH) {
				return os.ErrProcessDone
			}
			return err
		}
		return nil
	}
	cmd.WaitDelay = runGrace
}

// quotedPath pulls the path out of a line svn wrote about one, which it always
// names in quotes: "Reverted 'a.txt'", "Skipped missing target: 'a.txt'". It
// reports false for a line naming none, which is most of them.
func quotedPath(line string) (string, bool) {
	first, last := strings.Index(line, "'"), strings.LastIndex(line, "'")
	if first < 0 || last <= first {
		return "", false
	}
	return line[first+1 : last], true
}
