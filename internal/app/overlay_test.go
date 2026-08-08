package app

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"

	"github.com/bapatchirag/revision/internal/selfupdate"
	"github.com/bapatchirag/revision/internal/svn"
)

// TestLongErrorToastStaysOnScreen guards the composed view against a raw svn
// error: the notice box must wrap inside the terminal rather than draw past its
// right edge, which the terminal would wrap and the border break.
func TestLongErrorToastStaysOnScreen(t *testing.T) {
	m := sizedModel(t)
	err := errors.New("svn: E155007: '/home/alice/work/wc/some/deeply/nested/path/to/a/file.go' is not a working copy; run 'svn cleanup' and try again")
	next, _ := m.Update(committedMsg{err: err})
	m = next.(*Model)

	view := m.View()
	if !strings.Contains(stripANSI(view), "E155007") {
		t.Fatalf("expected the failure toast, got:\n%s", stripANSI(view))
	}
	for i, ln := range strings.Split(view, "\n") {
		if w := ansi.StringWidth(ln); w > m.width {
			t.Errorf("row %d width = %d, wider than the %d-column terminal:\n%s", i, w, m.width, stripANSI(ln))
		}
	}
}

func TestToastDismissedOnKey(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "a.go", State: svn.StateModified},
		{Path: "b.go", State: svn.StateModified},
	})
	next, _ := m.Update(committedMsg{revision: "9"})
	m = next.(*Model)
	if view := stripANSI(m.View()); !strings.Contains(view, "committed r9") {
		t.Fatalf("expected the commit toast, got:\n%s", view)
	}
	// Any interaction (here: navigating the Files panel) clears the toast.
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = next.(*Model)
	if view := stripANSI(m.View()); strings.Contains(view, "committed r9") {
		t.Errorf("the toast should clear on the next key, got:\n%s", view)
	}
}

func TestModalConfirmGolden(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "internal/app/app.go", State: svn.StateModified},
	})
	// The cursor opens on the app.go leaf (the tree skips the / root and the
	// internal/ and app/ directory rows), so delete targets the file directly.
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	m = next.(*Model)
	golden.RequireEqual(t, []byte(m.View()))
}

func TestHelpMenuOpensAndCloses(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "modified.go", State: svn.StateModified},
	})

	// "?" floats the keybindings menu over the layout.
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	m = next.(*Model)
	if !m.helping {
		t.Fatal("expected the help menu to open on ?")
	}
	view := stripANSI(m.View())
	for _, want := range []string{"Keybindings", "Stage / unstage", "space", "Quit"} {
		if !strings.Contains(view, want) {
			t.Errorf("help view missing %q\n---\n%s", want, view)
		}
	}

	// While help is open, other keys are captured by the menu — q must not quit.
	if _, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}}); cmd != nil {
		if _, ok := cmd().(tea.QuitMsg); ok {
			t.Error("q should not quit while the help menu is open")
		}
	}
	if !m.helping {
		t.Error("the help menu should stay open on a non-dismiss key")
	}

	// enter must NOT close the help menu — it is a read-only reference.
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(*Model)
	if cmd != nil {
		next, _ = m.Update(cmd()) // deliver the resulting ActivatedMsg
		m = next.(*Model)
	}
	if !m.helping {
		t.Error("enter should not close the help menu")
	}

	// esc closes the help menu.
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(*Model)
	if m.helping {
		t.Error("the help menu should close after esc")
	}
	if view := stripANSI(m.View()); strings.Contains(view, "Keybindings") {
		t.Error("the layout should return after closing help")
	}
}

func TestHelpMenuTogglesClosedWithQuestionMark(t *testing.T) {
	m := loadItems(t, sizedModel(t), nil)
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	m = next.(*Model)
	if !m.helping {
		t.Fatal("? should open the help menu")
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	m = next.(*Model)
	if m.helping {
		t.Error("? should toggle the help menu closed")
	}
}

func TestAuthFailureShowsHint(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "modified.go", State: svn.StateModified, Changelist: "revision:staged"},
	})
	authErr := errors.New("svn commit: E170001: No more credentials or we tried too many times.")
	next, _ := m.Update(committedMsg{err: authErr})
	m = next.(*Model)

	view := stripANSI(m.View())
	if !strings.Contains(view, "authentication required") {
		t.Errorf("expected an auth hint toast, got:\n%s", view)
	}
	if strings.Contains(view, "E170001") {
		t.Errorf("the raw svn error should be replaced by the hint, got:\n%s", view)
	}
}

func TestHelpMenuGolden(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "modified.go", State: svn.StateModified},
	})
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	m = next.(*Model)
	golden.RequireEqual(t, []byte(m.View()))
}

// availableRelease is the fixture release the update-prompt tests offer.
var availableRelease = selfupdate.Release{Tag: "v1.5.0", Version: "1.5.0", URL: "https://example.test/r"}

func TestUpdatePromptOpensOnAvailable(t *testing.T) {
	m := loadItems(t, sizedModel(t), nil)
	next, _ := m.Update(updateAvailableMsg{rel: availableRelease})
	m = next.(*Model)

	if !m.updating {
		t.Fatal("expected the update prompt to open when a newer release is available")
	}
	view := stripANSI(m.View())
	for _, want := range []string{"Update available: v1.5.0", "Update with cURL", "Update with Go", "Don't update this time"} {
		if !strings.Contains(view, want) {
			t.Errorf("update prompt missing %q\n---\n%s", want, view)
		}
	}
}

func TestUpdatePromptCurlQuitsWithPendingMethod(t *testing.T) {
	m := loadItems(t, sizedModel(t), nil)
	next, _ := m.Update(updateAvailableMsg{rel: availableRelease})
	m = next.(*Model)

	// enter on the first item (Update with cURL) emits an ActivatedMsg…
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(*Model)
	if cmd == nil {
		t.Fatal("expected a command from selecting a menu item")
	}
	// …which, once delivered, records the method and quits.
	next, cmd = m.Update(cmd())
	m = next.(*Model)
	if cmd == nil {
		t.Fatal("expected a quit command after choosing an update method")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatalf("expected tea.QuitMsg, got %T", cmd())
	}
	method, rel, chosen := m.PendingUpdate()
	if !chosen || method != selfupdate.MethodCurl {
		t.Errorf("PendingUpdate() = (%v, %v), want (curl, true)", method, chosen)
	}
	if rel != availableRelease {
		t.Errorf("PendingUpdate() release = %+v, want %+v", rel, availableRelease)
	}
}

func TestUpdatePromptGoQuitsWithPendingMethod(t *testing.T) {
	m := loadItems(t, sizedModel(t), nil)
	next, _ := m.Update(updateAvailableMsg{rel: availableRelease})
	m = next.(*Model)

	// Move to the second item (Update with Go), then select it.
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = next.(*Model)
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(*Model)
	next, _ = m.Update(cmd())
	m = next.(*Model)

	method, rel, chosen := m.PendingUpdate()
	if !chosen || method != selfupdate.MethodGo {
		t.Errorf("PendingUpdate() = (%v, %v), want (go, true)", method, chosen)
	}
	if rel != availableRelease {
		t.Errorf("PendingUpdate() release = %+v, want %+v", rel, availableRelease)
	}
}

func TestUpdatePromptDeclineDismissesWithoutUpdate(t *testing.T) {
	m := loadItems(t, sizedModel(t), nil)
	next, _ := m.Update(updateAvailableMsg{rel: availableRelease})
	m = next.(*Model)

	// Third item is "Don't update this time": it closes the prompt and records
	// no pending update.
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = next.(*Model)
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = next.(*Model)
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(*Model)
	if cmd != nil {
		next, _ = m.Update(cmd())
		m = next.(*Model)
	}
	if m.updating {
		t.Error("the prompt should close after declining")
	}
	if _, _, chosen := m.PendingUpdate(); chosen {
		t.Error("declining must not record a pending update")
	}
}

func TestUpdatePromptEscDismisses(t *testing.T) {
	m := loadItems(t, sizedModel(t), nil)
	next, _ := m.Update(updateAvailableMsg{rel: availableRelease})
	m = next.(*Model)

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(*Model)
	if cmd != nil {
		next, _ = m.Update(cmd()) // deliver the DismissMsg
		m = next.(*Model)
	}
	if m.updating {
		t.Error("esc should dismiss the update prompt")
	}
	if _, _, chosen := m.PendingUpdate(); chosen {
		t.Error("dismissing must not record a pending update")
	}
	if view := stripANSI(m.View()); strings.Contains(view, "Update available") {
		t.Error("the layout should return after dismissing the update prompt")
	}
}

func TestUpdatePromptSuppressedWhileOverlayActive(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "modified.go", State: svn.StateModified, Changelist: "revision:staged"},
	})
	// Open the commit editor, then let the update check land underneath it.
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	m = next.(*Model)
	if !m.editing {
		t.Fatal("expected the commit editor to be open")
	}
	next, _ = m.Update(updateAvailableMsg{rel: availableRelease})
	m = next.(*Model)
	if m.updating {
		t.Error("the update prompt must not steal focus from an active overlay")
	}
}

func TestUpdatePromptGolden(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "modified.go", State: svn.StateModified},
	})
	next, _ := m.Update(updateAvailableMsg{rel: availableRelease})
	m = next.(*Model)
	golden.RequireEqual(t, []byte(m.View()))
}
