package app

import (
	"reflect"
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
	for _, want := range []string{"Settings", "Log limit", "Editor", "Theme", "Directory diff", "Hide untracked", "SSH key", "Display from", "Diff output", "Hide rules"} {
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

// stepKey applies a key and delivers the one follow-up message the focused
// overlay's command produces — activate, submit or dismiss — as the runtime
// would, so its effect lands within the step.
func stepKey(t *testing.T, m *Model, k tea.KeyMsg) *Model {
	t.Helper()
	next, cmd := m.Update(k)
	m = next.(*Model)
	if cmd != nil {
		if out := cmd(); out != nil {
			next, _ = m.Update(out)
			m = next.(*Model)
		}
	}
	return m
}

// openRulesEditor opens the settings editor and activates its Hide rules row.
func openRulesEditor(t *testing.T, m *Model) *Model {
	t.Helper()
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'S'}})
	m = pressDown(t, next.(*Model), hideRulesFieldIndex)
	return stepKey(t, m, tea.KeyMsg{Type: tea.KeyEnter})
}

// addRule types a pattern into a new row of the open rules editor and keeps it.
func addRule(t *testing.T, m *Model, pattern string) *Model {
	t.Helper()
	m = stepKey(t, m, keyRunes("a"))
	m = stepKey(t, m, keyRunes(pattern))
	return stepKey(t, m, tea.KeyMsg{Type: tea.KeyEnter})
}

func TestHideRulesEditorOpensOverSettings(t *testing.T) {
	m := openRulesEditor(t, sizedModel(t))

	if !m.editingRules {
		t.Fatal("activating the Hide rules row did not open the editor")
	}
	if !m.configuring {
		t.Error("the settings editor should stay open beneath the rules editor")
	}
	view := stripANSI(m.View())
	for _, want := range []string{"Hide rules", "a add", "Settings"} {
		if !strings.Contains(view, want) {
			t.Errorf("view missing %q\n---\n%s", want, view)
		}
	}
}

func TestHideRulesEditorEscReturnsToSettings(t *testing.T) {
	m := openRulesEditor(t, sizedModel(t))
	m = stepKey(t, m, tea.KeyMsg{Type: tea.KeyEsc})

	if m.editingRules {
		t.Error("esc should close the rules editor")
	}
	if !m.configuring {
		t.Error("esc in the rules editor should leave the settings editor open")
	}
	// The form has the keyboard back, so its own esc closes it.
	m = stepKey(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.configuring {
		t.Error("esc should then close the settings editor")
	}
}

func TestHideRulesEditorSavesRules(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	m := openRulesEditor(t, sizedModel(t))

	m = addRule(t, m, "^build/")
	m = stepKey(t, m, tea.KeyMsg{Type: tea.KeyCtrlS})
	if m.editingRules {
		t.Fatal("ctrl+s should close the rules editor")
	}
	if got, want := m.form.Value(hideRulesFieldIndex), "1 rule · 1 on"; got != want {
		t.Errorf("summary = %q, want %q", got, want)
	}
	// The rules are only written when the settings editor itself is saved.
	if len(m.cfg.HideRules) != 0 {
		t.Fatalf("cfg.HideRules = %+v, want none until the form is saved", m.cfg.HideRules)
	}

	m = stepKey(t, m, tea.KeyMsg{Type: tea.KeyCtrlS})
	want := []config.HideRule{{Pattern: "^build/", Enabled: true}}
	if !reflect.DeepEqual(m.cfg.HideRules, want) {
		t.Errorf("cfg.HideRules = %+v, want %+v", m.cfg.HideRules, want)
	}
	got, err := config.Load()
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	if !reflect.DeepEqual(got.HideRules, want) {
		t.Errorf("persisted HideRules = %+v, want %+v", got.HideRules, want)
	}
}

func TestHideRulesApplyWhenSettingsAreSaved(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "build/gen.go", State: svn.StateModified},
		{Path: "src/a.go", State: svn.StateModified},
	})

	m = openRulesEditor(t, m)
	m = addRule(t, m, "^build/")
	m = stepKey(t, m, tea.KeyMsg{Type: tea.KeyCtrlS}) // keep the rules
	if !fileTreeHasPath(m, "build/gen.go") {
		t.Fatal("nothing should be hidden before the settings are saved")
	}

	m = stepKey(t, m, tea.KeyMsg{Type: tea.KeyCtrlS}) // save the settings
	if fileTreeHasPath(m, "build/gen.go") {
		t.Error("saving should hide the matching file without a status reload")
	}
	if !fileTreeHasPath(m, "src/a.go") {
		t.Error("the file no rule matches should still be there")
	}
}

func TestHideRulesEditorRejectsAnInvalidPattern(t *testing.T) {
	m := openRulesEditor(t, sizedModel(t))

	m = addRule(t, m, "[unclosed")
	m = stepKey(t, m, tea.KeyMsg{Type: tea.KeyCtrlS})

	if !m.editingRules {
		t.Error("the editor should stay open on a pattern that cannot compile")
	}
	if len(m.rulesDraft) != 0 {
		t.Errorf("rulesDraft = %+v, want none (the rule was rejected)", m.rulesDraft)
	}
	if view := stripANSI(m.View()); !strings.Contains(view, "invalid pattern: [unclosed") {
		t.Errorf("view missing the rejection notice\n---\n%s", view)
	}
}

func TestHideRulesDiscardedWhenSettingsCancelled(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfg := config.Default()
	cfg.HideRules = []config.HideRule{{Pattern: "^build/", Enabled: true}}
	m := openRulesEditor(t, sizedModelCfg(t, cfg))

	// Delete the existing rule, keep the edit, then abandon the settings editor.
	m = stepKey(t, m, keyRunes("d"))
	m = stepKey(t, m, tea.KeyMsg{Type: tea.KeyCtrlS})
	m = stepKey(t, m, tea.KeyMsg{Type: tea.KeyEsc})

	if m.configuring {
		t.Fatal("esc should close the settings editor")
	}
	if !reflect.DeepEqual(m.cfg.HideRules, cfg.HideRules) {
		t.Errorf("cfg.HideRules = %+v, want %+v (cancelling discards the edit)", m.cfg.HideRules, cfg.HideRules)
	}
	if m.rulesDraft != nil {
		t.Errorf("rulesDraft = %+v, want it dropped with the editor", m.rulesDraft)
	}
}
