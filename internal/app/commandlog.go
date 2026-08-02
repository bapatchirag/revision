package app

import (
	"strings"
	"sync"

	"github.com/charmbracelet/lipgloss"

	"github.com/bapatchirag/revision/internal/svn"
)

// commandLogLimit is how many recent svn invocations the command log keeps.
const commandLogLimit = 100

// readOnlyCommands are the svn subcommands revision runs on its own to read
// state. The command log omits them so it shows only the actions the user
// performed — unless the invocation was one the user asked for, which svn
// reports through CommandRecord.UserAction.
var readOnlyCommands = map[string]bool{
	"diff":   true,
	"status": true,
	"log":    true,
	"info":   true,
}

// isReadOnlyCommand reports whether sub is one of the automatic read-only
// queries the command log should not show.
func isReadOnlyCommand(sub string) bool { return readOnlyCommands[sub] }

// loggedCommand reports whether an invocation belongs in the command log: every
// action the user performed, including a read-only query they asked for — the
// `svn diff` behind writing a patch out — but none of the ones revision runs on
// its own to fill a panel.
func loggedCommand(r svn.CommandRecord) bool {
	return r.UserAction || !isReadOnlyCommand(r.Subcommand)
}

// commandLog is a bounded, concurrency-safe ring of the svn invocations revision
// has run. svn commands complete on background goroutines, so record may be
// called concurrently; every read and write is guarded by the mutex.
type commandLog struct {
	mu      sync.Mutex
	entries []svn.CommandRecord
	total   int64
	limit   int
}

// newCommandLog returns a command log that retains at most limit entries.
func newCommandLog(limit int) *commandLog {
	if limit <= 0 {
		limit = commandLogLimit
	}
	return &commandLog{limit: limit}
}

// record appends one invocation, dropping the oldest entries once the ring is
// full. Output is discarded: the panel shows only the command and whether it
// succeeded, and Err is kept solely as that success/failure signal.
func (l *commandLog) record(r svn.CommandRecord) {
	r.Output = ""

	l.mu.Lock()
	defer l.mu.Unlock()
	l.entries = append(l.entries, r)
	if len(l.entries) > l.limit {
		n := copy(l.entries, l.entries[len(l.entries)-l.limit:])
		l.entries = l.entries[:n]
	}
	l.total++
}

// snapshot returns a copy of the retained entries, oldest first.
func (l *commandLog) snapshot() []svn.CommandRecord {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]svn.CommandRecord, len(l.entries))
	copy(out, l.entries)
	return out
}

// seq returns the total number of invocations ever recorded. It only grows, so
// the UI can detect new commands even after the ring has started dropping old
// ones (where the entry count stops changing).
func (l *commandLog) seq() int64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.total
}

// clear drops every retained entry, releasing the log at shutdown.
func (l *commandLog) clear() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.entries = nil
}

// syncCommandLog refreshes the command-log viewport from the recorder when a new
// command has been logged since the last refresh. It is cheap to call on every
// message: the sequence check skips the rebuild when nothing changed, which also
// leaves any manual scroll position untouched between commands.
func (m *Model) syncCommandLog() {
	if m.cmdLog == nil || m.cmdLogView == nil {
		return
	}
	seq := m.cmdLog.seq()
	if seq == m.cmdLogSeen {
		return
	}
	m.cmdLogSeen = seq
	m.cmdLogView.SetContent(m.renderCommandLog(m.cmdLog.snapshot()))
}

// renderCommandLog formats the recorded invocations for the command-log panel,
// newest first. Each entry is a single line: a success or failure marker
// followed by the command that ran.
func (m *Model) renderCommandLog(entries []svn.CommandRecord) string {
	if len(entries) == 0 {
		return lipgloss.NewStyle().Foreground(m.theme.Muted).Render("No svn commands run yet.")
	}

	pass := lipgloss.NewStyle().Foreground(m.theme.Success)
	fail := lipgloss.NewStyle().Foreground(m.theme.Error)
	cmd := lipgloss.NewStyle().Foreground(m.theme.Text)

	var b strings.Builder
	for i := len(entries) - 1; i >= 0; i-- {
		e := entries[i]
		if b.Len() > 0 {
			b.WriteByte('\n')
		}
		mark := pass.Render("✓")
		if e.Err != "" {
			mark = fail.Render("✗")
		}
		b.WriteString(mark + " " + cmd.Render(e.Command))
	}
	return b.String()
}
