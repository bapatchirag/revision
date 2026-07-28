package svn

import (
	"context"
	"os/exec"
	"strings"
	"testing"
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
