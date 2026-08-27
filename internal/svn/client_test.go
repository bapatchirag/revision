package svn

import (
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestRunRecordsSuccessfulCommand(t *testing.T) {
	if _, err := exec.LookPath("echo"); err != nil {
		t.Skip("echo not found on PATH")
	}
	var got CommandRecord
	calls := 0
	c := &Client{Bin: "echo", Recorder: func(r CommandRecord) {
		got = r
		calls++
	}}

	if _, err := c.run(context.Background(), "hello"); err != nil {
		t.Fatalf("run: %v", err)
	}
	if calls != 1 {
		t.Fatalf("recorder called %d times, want 1", calls)
	}
	if !strings.Contains(got.Command, "echo hello --non-interactive") {
		t.Errorf("Command = %q, want the full invocation", got.Command)
	}
	if !strings.Contains(got.Output, "hello") {
		t.Errorf("Output = %q, want the echoed text", got.Output)
	}
	if got.Err != "" {
		t.Errorf("Err = %q, want empty on success", got.Err)
	}
}

func TestRunRecordsFailedCommand(t *testing.T) {
	if _, err := exec.LookPath("false"); err != nil {
		t.Skip("false not found on PATH")
	}
	var got CommandRecord
	c := &Client{Bin: "false", Recorder: func(r CommandRecord) { got = r }}

	if _, err := c.run(context.Background(), "boom"); err == nil {
		t.Fatal("expected run to fail with the false stub")
	}
	if !strings.Contains(got.Command, "false boom --non-interactive") {
		t.Errorf("Command = %q, want the full invocation even on failure", got.Command)
	}
	if got.Err == "" {
		t.Error("Err should be populated when the command fails")
	}
}

func TestRunWithoutRecorderSucceeds(t *testing.T) {
	if _, err := exec.LookPath("echo"); err != nil {
		t.Skip("echo not found on PATH")
	}
	c := &Client{Bin: "echo"} // no Recorder set
	if _, err := c.run(context.Background(), "hello"); err != nil {
		t.Fatalf("run without a recorder should still work: %v", err)
	}
}

func TestRunReportsTimeoutRatherThanSignal(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh not found on PATH")
	}
	var got CommandRecord
	// sh -c swallows the appended --non-interactive as $0, so the stub still sleeps.
	c := &Client{Bin: "sh", Recorder: func(r CommandRecord) { got = r }}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := c.run(ctx, "-c", "sleep 10")
	if err == nil {
		t.Fatal("expected run to fail once the deadline expired")
	}
	if !strings.Contains(err.Error(), "timed out after") {
		t.Errorf("error = %q, want the deadline reported as a timeout", err)
	}
	if strings.Contains(err.Error(), "signal: killed") {
		t.Errorf("error = %q, should not leak the kill signal", err)
	}
	if !strings.Contains(got.Err, "timed out after") {
		t.Errorf("record Err = %q, want the same timeout text", got.Err)
	}
}

func TestRunDoesNotReportCancellationAsTimeout(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh not found on PATH")
	}
	c := &Client{Bin: "sh"}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := c.run(ctx, "-c", "sleep 10")
	if err == nil {
		t.Fatal("expected run to fail once the context was cancelled")
	}
	if strings.Contains(err.Error(), "timed out after") {
		t.Errorf("error = %q, a cancellation is not a timeout", err)
	}
}

// TestRunReturnsWhenAChildOutlivesTheKill is the one that makes a timeout
// recoverable. svn over svn+ssh forks ssh, which inherits the output pipes; the
// deadline kills svn alone, so without the process group and the wait delay the
// surviving child holds the pipe open and run blocks forever — the reply never
// reaches the UI, and refreshing only stacks up another stuck read.
func TestRunReturnsWhenAChildOutlivesTheKill(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh not found on PATH")
	}
	c := &Client{Bin: "sh"}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		// The backgrounded sleep inherits stdout and is not the process the
		// deadline kills.
		_, err := c.run(ctx, "-c", "sleep 60 & sleep 60")
		done <- err
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected the overrun to fail")
		}
		if !strings.Contains(err.Error(), "timed out after") {
			t.Errorf("error = %q, want the deadline reported as a timeout", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("run never returned: a child outliving the kill still holds the output pipe")
	}
}

// TestRunSucceedsWhenAChildLingersPastExit is the other side of the wait delay:
// svn ran to completion and its output is whole, so a child still holding the
// pipe open behind it must not turn that into a failure.
func TestRunSucceedsWhenAChildLingersPastExit(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh not found on PATH")
	}
	c := &Client{Bin: "sh"}

	out, err := c.run(context.Background(), "-c", "echo hello; sleep 60 &")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(string(out), "hello") {
		t.Errorf("output = %q, want the command's own output", out)
	}
}
