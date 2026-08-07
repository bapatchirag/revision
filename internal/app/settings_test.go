package app

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/golden"

	"github.com/bapatchirag/revision/internal/config"
	"github.com/bapatchirag/revision/internal/svn"
	"github.com/bapatchirag/revision/internal/tui/theme"
)

func TestSettingsSwitchesDisplayScope(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	m := subdirModel(t, config.Default())

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'S'}})
	m = next.(*Model)
	for i := 0; i < 6; i++ {
		next, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown}) // to the Display from field
		m = next.(*Model)
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRight}) // cwd -> root
	m = next.(*Model)

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	m = next.(*Model)
	if cmd != nil {
		next, _ = m.Update(cmd())
		m = next.(*Model)
	}

	if m.cfg.DisplayFrom != config.DisplayFromRoot {
		t.Fatalf("cfg.DisplayFrom = %q, want %q", m.cfg.DisplayFrom, config.DisplayFromRoot)
	}
	if m.client.Dir != "/home/alice/work/wc" {
		t.Errorf("client.Dir = %q, want the working-copy root after switching scope", m.client.Dir)
	}
	got, err := config.Load()
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	if got.DisplayFrom != config.DisplayFromRoot {
		t.Errorf("persisted DisplayFrom = %q, want %q", got.DisplayFrom, config.DisplayFromRoot)
	}
}

func TestSettingsSavesHideUntrackedToggle(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "modified.go", State: svn.StateModified},
		{Path: "scratch.txt", State: svn.StateUnversioned},
	})
	if m.hideUntracked {
		t.Fatal("hide-untracked should start disabled by default")
	}

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'S'}})
	m = next.(*Model)
	for i := 0; i < 4; i++ {
		next, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown}) // to the Hide untracked field
		m = next.(*Model)
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeySpace}) // toggle on
	m = next.(*Model)

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	m = next.(*Model)
	if cmd != nil {
		next, _ = m.Update(cmd())
		m = next.(*Model)
	}
	if !m.hideUntracked {
		t.Error("saving did not apply the hide-untracked toggle")
	}
	if fileTreeHasPath(m, "scratch.txt") {
		t.Error("untracked file should be hidden immediately after saving hide-untracked")
	}
	got, err := config.Load()
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	if !got.HideUntracked {
		t.Error("persisted HideUntracked = false, want true")
	}
}

func TestToggleUntrackedNotCapturedBySettingsSave(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "modified.go", State: svn.StateModified},
		{Path: "scratch.txt", State: svn.StateUnversioned},
	})

	// Hide untracked for the session via the keybind.
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'U'}})
	m = next.(*Model)
	if !m.hideUntracked {
		t.Fatal("pressing U did not hide untracked files for the session")
	}

	// Open settings and save without touching the hide-untracked field. The form
	// mirrors the persisted config (untracked still shown), so the save must not
	// capture the session toggle.
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'S'}})
	m = next.(*Model)
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	m = next.(*Model)
	if cmd != nil {
		next, _ = m.Update(cmd())
		m = next.(*Model)
	}
	got, err := config.Load()
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	if got.HideUntracked {
		t.Error("saving settings captured the session toggle; persisted HideUntracked should stay false")
	}
	if m.cfg.HideUntracked {
		t.Error("in-memory config HideUntracked should stay false after saving unrelated settings")
	}
	// Saving resets the session view to the persisted default, so untracked show again.
	if m.hideUntracked {
		t.Error("saving settings should reset the session toggle to the persisted default")
	}
	if !fileTreeHasPath(m, "scratch.txt") {
		t.Error("untracked file should reappear after the save reset the session toggle")
	}
}

func TestSettingsLivePreviewOnScroll(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "modified.go", State: svn.StateModified},
	})
	if m.theme != theme.Auto() {
		t.Fatal("initial theme is not Auto()")
	}

	// S opens the settings editor; the Theme field starts on the active theme.
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'S'}})
	m = next.(*Model)
	if !m.configuring {
		t.Fatal("pressing S did not open the settings editor")
	}

	// Navigate down to the Theme field; moving between fields must not preview.
	for i := 0; i < themeFieldIndex; i++ {
		next, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
		m = next.(*Model)
	}
	if m.theme != theme.Auto() {
		t.Error("navigating to the Theme field should not change the palette")
	}

	// Cycling the Theme field forward live-applies the highlighted theme
	// (auto -> everforest) without persisting the choice.
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRight})
	m = next.(*Model)
	if m.theme != theme.Everforest() {
		t.Error("cycling the Theme field did not live-apply Everforest()")
	}
	if m.cfg.Theme != "auto" {
		t.Errorf("preview must not persist; cfg.Theme = %q, want auto", m.cfg.Theme)
	}

	// Esc cancels, reverting the live preview to the original theme.
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(*Model)
	if cmd != nil {
		next, _ = m.Update(cmd()) // deliver the DismissMsg
		m = next.(*Model)
	}
	if m.theme != theme.Auto() {
		t.Error("esc did not revert the live preview to the original theme")
	}
	if m.configuring {
		t.Error("esc did not close the settings editor")
	}
}

func TestSettingsOpensAndCancels(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "modified.go", State: svn.StateModified},
	})

	// S floats the settings editor over the layout.
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'S'}})
	m = next.(*Model)
	if !m.configuring {
		t.Fatal("pressing S did not open the settings editor")
	}
	view := stripANSI(m.View())
	for _, want := range []string{"Settings", "Log limit", "Editor", "Theme", "Directory diff", "Hide untracked", "SSH key", "Display from", "Diff output"} {
		if !strings.Contains(view, want) {
			t.Errorf("settings view missing %q\n---\n%s", want, view)
		}
	}

	// esc closes it without saving.
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(*Model)
	if cmd != nil {
		next, _ = m.Update(cmd()) // deliver the DismissMsg
		m = next.(*Model)
	}
	if m.configuring {
		t.Error("esc should close the settings editor")
	}
	if view := stripANSI(m.View()); strings.Contains(view, "Directory diff") {
		t.Error("the layout should return after closing settings")
	}
}

func TestSettingsSavesThemeChange(t *testing.T) {
	// Persist to a throwaway XDG config dir so the real home is untouched.
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "modified.go", State: svn.StateModified},
	})
	if m.theme != theme.Auto() {
		t.Fatal("initial theme is not Auto()")
	}

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'S'}})
	m = next.(*Model)

	// Move to the Theme field and cycle one option forward (auto -> everforest).
	for i := 0; i < 2; i++ {
		next, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
		m = next.(*Model)
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRight})
	m = next.(*Model)

	// ctrl+s saves and closes.
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	m = next.(*Model)
	if cmd != nil {
		next, _ = m.Update(cmd()) // deliver the SubmitMsg
		m = next.(*Model)
	}
	if m.configuring {
		t.Fatal("ctrl+s should close the settings editor")
	}
	if m.theme != theme.Everforest() {
		t.Error("saving did not apply the chosen theme")
	}
	if m.cfg.Theme != "everforest" {
		t.Errorf("cfg.Theme = %q, want everforest", m.cfg.Theme)
	}
	got, err := config.Load()
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	if got.Theme != "everforest" {
		t.Errorf("persisted theme = %q, want everforest", got.Theme)
	}
	if view := stripANSI(m.View()); !strings.Contains(view, "settings saved") {
		t.Errorf("expected a saved toast, got:\n%s", view)
	}
}

func TestSettingsSavesDirectoryDiffToggle(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "modified.go", State: svn.StateModified},
	})
	if !m.dirDiff {
		t.Fatal("directory diff should start enabled by default")
	}

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'S'}})
	m = next.(*Model)
	for i := 0; i < 3; i++ {
		next, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown}) // to the Directory diff field
		m = next.(*Model)
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeySpace}) // toggle off
	m = next.(*Model)

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	m = next.(*Model)
	if cmd != nil {
		next, _ = m.Update(cmd())
		m = next.(*Model)
	}
	if m.dirDiff {
		t.Error("saving did not apply the directory-diff toggle")
	}
	got, err := config.Load()
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	if got.DirectoryDiff {
		t.Error("persisted DirectoryDiff = true, want false")
	}
}

func TestSettingsEditsPersistToConfig(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	m := sizedModel(t)

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'S'}})
	m = next.(*Model)
	// Move to the Editor field and cycle it forward to nvim (native -> vim -> nvim).
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = next.(*Model)
	for i := 0; i < 2; i++ {
		next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRight})
		m = next.(*Model)
	}
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	m = next.(*Model)
	if cmd != nil {
		m.Update(cmd()) // deliver the SubmitMsg; persisting is a side effect
	}
	got, err := config.Load()
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	if got.Editor != "nvim" {
		t.Errorf("persisted Editor = %q, want nvim", got.Editor)
	}
}

func TestSettingsFormGolden(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "modified.go", State: svn.StateModified},
	})
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'S'}})
	m = next.(*Model)
	golden.RequireEqual(t, []byte(m.View()))
}
