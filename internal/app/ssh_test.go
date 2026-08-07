package app

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/bapatchirag/revision/internal/config"
	"github.com/bapatchirag/revision/internal/selfupdate"
	"github.com/bapatchirag/revision/internal/sshagent"
	"github.com/bapatchirag/revision/internal/svn"
	uimsg "github.com/bapatchirag/revision/internal/tui/msg"
)

func sshModel(t *testing.T) *Model {
	t.Helper()
	m := New(nil, &svn.Info{URL: "svn+ssh://host/repo/trunk", Revision: "7"}, selfupdate.Build{}, config.Default())
	next, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	return next.(*Model)
}

func TestNeedsSSHKeyFromURL(t *testing.T) {
	https := New(nil, &svn.Info{URL: "https://h/r"}, selfupdate.Build{}, config.Default())
	if https.needsSSHKey {
		t.Error("an https working copy should not need an SSH key")
	}
	ssh := New(nil, &svn.Info{URL: "svn+ssh://h/r"}, selfupdate.Build{}, config.Default())
	if !ssh.needsSSHKey {
		t.Error("an svn+ssh working copy should need an SSH key")
	}
}

func TestSSHKeyLoadedProceedsWithoutPrompt(t *testing.T) {
	m := sshModel(t)
	next, cmd := m.Update(sshCheckedMsg{loaded: true})
	m = next.(*Model)
	if m.unlocking {
		t.Error("a key already in the agent should not open the passphrase prompt")
	}
	if cmd == nil {
		t.Error("expected the initial load to start once the key is ready")
	}
	if view := stripANSI(m.View()); strings.Contains(view, "SSH key passphrase") {
		t.Errorf("passphrase overlay should not be shown, got:\n%s", view)
	}
}

func TestSSHKeyMissingOpensPrompt(t *testing.T) {
	m := sshModel(t)
	next, _ := m.Update(sshCheckedMsg{loaded: false})
	m = next.(*Model)
	if !m.unlocking {
		t.Fatal("a key missing from the agent should open the passphrase prompt")
	}
	if view := stripANSI(m.View()); !strings.Contains(view, "SSH key passphrase") {
		t.Errorf("passphrase overlay missing, got:\n%s", view)
	}
}

func isQuit(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	_, ok := cmd().(tea.QuitMsg)
	return ok
}

// assertAbortThenQuit checks that m is showing the centered fatal-error toast
// with the quit hint, has recorded an abort reason, and quits on the next key.
func assertAbortThenQuit(t *testing.T, m *Model) {
	t.Helper()
	if !m.aborting {
		t.Fatal("expected the model to enter the abort state")
	}
	if view := stripANSI(m.View()); !strings.Contains(view, "Press any key to quit and try again") {
		t.Errorf("expected the quit hint in a centered toast, got:\n%s", view)
	}
	if _, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")}); !isQuit(cmd) {
		t.Error("any key should quit after an abort")
	}
}

func TestSSHCheckErrorAborts(t *testing.T) {
	m := sshModel(t)
	next, cmd := m.Update(sshCheckedMsg{err: errors.New("agent unreachable")})
	m = next.(*Model)
	if m.unlocking {
		t.Error("an agent error should not open the prompt")
	}
	if cmd != nil {
		t.Error("an abort should wait for a keypress, not quit immediately")
	}
	assertAbortThenQuit(t, m)
}

func TestSSHPassphraseMaskedInOverlay(t *testing.T) {
	m := sshModel(t)
	next, _ := m.Update(sshCheckedMsg{loaded: false})
	m = next.(*Model)
	// Type a passphrase through the model; it must never appear on screen.
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("hunter2")})
	m = next.(*Model)
	view := stripANSI(m.View())
	if strings.Contains(view, "hunter2") {
		t.Errorf("passphrase leaked into the view:\n%s", view)
	}
	if !strings.Contains(view, "\u2022") {
		t.Errorf("expected masked bullets in the overlay, got:\n%s", view)
	}
}

func TestSSHAddWrongPassphraseAllowsRetry(t *testing.T) {
	m := sshModel(t)
	next, _ := m.Update(sshCheckedMsg{loaded: false})
	m = next.(*Model)
	next, _ = m.Update(sshAddedMsg{err: errors.New("bad passphrase")})
	m = next.(*Model)
	if !m.unlocking {
		t.Error("a wrong passphrase should keep the prompt open for another try")
	}
	if m.adding {
		t.Error("the input should be unlocked again after a failed attempt")
	}
	if view := stripANSI(m.View()); !strings.Contains(view, "wrong passphrase") {
		t.Errorf("expected a wrong-passphrase toast, got:\n%s", view)
	}
}

func TestSSHTooManyBadPassphrasesAborts(t *testing.T) {
	m := sshModel(t)
	next, _ := m.Update(sshCheckedMsg{loaded: false})
	m = next.(*Model)

	for i := 0; i < maxPassphraseAttempts; i++ {
		next, _ = m.Update(sshAddedMsg{err: errors.New("bad passphrase")})
		m = next.(*Model)
	}
	assertAbortThenQuit(t, m)
}

func TestSSHAddAgentErrorAborts(t *testing.T) {
	m := sshModel(t)
	next, _ := m.Update(sshCheckedMsg{loaded: false})
	m = next.(*Model)
	next, _ = m.Update(sshAddedMsg{err: sshagent.ErrAgentUnreachable})
	m = next.(*Model)
	assertAbortThenQuit(t, m)
}

func TestSSHAddSuccessClosesPromptAndLoads(t *testing.T) {
	m := sshModel(t)
	next, _ := m.Update(sshCheckedMsg{loaded: false})
	m = next.(*Model)
	next, cmd := m.Update(sshAddedMsg{err: nil})
	m = next.(*Model)
	if m.unlocking {
		t.Error("a successful add should close the passphrase prompt")
	}
	if cmd == nil {
		t.Error("expected the initial load to start after the key is added")
	}
	if view := stripANSI(m.View()); !strings.Contains(view, "SSH key added") {
		t.Errorf("expected a success toast, got:\n%s", view)
	}
}

func TestSSHPromptDismissAborts(t *testing.T) {
	m := sshModel(t)
	next, _ := m.Update(sshCheckedMsg{loaded: false})
	m = next.(*Model)
	next, _ = m.Update(uimsg.DismissMsg{ID: passphraseEditorID})
	m = next.(*Model)
	assertAbortThenQuit(t, m)
}

func TestSSHSubmitLocksInputAndIndicates(t *testing.T) {
	m := sshModel(t)
	next, _ := m.Update(sshCheckedMsg{loaded: false})
	m = next.(*Model)
	next, cmd := m.Update(uimsg.SubmitMsg{ID: passphraseEditorID, Value: "pw"})
	m = next.(*Model)
	if !m.adding {
		t.Error("submitting should mark the add as in progress")
	}
	if m.passEditor.Focused() {
		t.Error("submitting should lock (blur) the passphrase input")
	}
	if cmd == nil {
		t.Error("submitting should start the ssh-add command")
	}
	if view := stripANSI(m.View()); !strings.Contains(view, "Adding SSH key") {
		t.Errorf("expected a processing indicator, got:\n%s", view)
	}
	// While the add is in flight, further key input is ignored.
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	m = next.(*Model)
	if m.passEditor.Value() != "" {
		t.Errorf("input should be locked while adding, got %q", m.passEditor.Value())
	}
}
