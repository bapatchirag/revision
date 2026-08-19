package app

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/bapatchirag/revision/internal/config"
	"github.com/bapatchirag/revision/internal/tui/component"
	"github.com/bapatchirag/revision/internal/tui/theme"
)

// previewTheme applies the named palette to every component without persisting,
// so the settings editor can show each scheme live while its Theme field cycles.
// It re-themes every component, rebuilds the Files list render closures that
// captured the previous palette (so row glyph colors follow the switch), and
// refreshes derived chrome (which re-colorizes the diff via the live theme). An
// unrecognized name resolves to Auto (matching startup), so it is always safe.
func (m *Model) previewTheme(name string) {
	th, _ := theme.ByName(name)
	m.theme = th
	m.themeName = name
	// Pin the color profile before re-theming so the palette (and the diff
	// re-colorized by refreshChrome) renders in the profile the theme expects.
	theme.ApplyColorProfile(name)
	for _, p := range m.panels {
		p.SetTheme(th)
	}
	m.bar.SetTheme(th)
	m.editor.SetTheme(th)
	m.nameEditor.SetTheme(th)
	m.diffEditor.SetTheme(th)
	m.pathEditor.SetTheme(th)
	m.repoEditor.SetTheme(th)
	m.modal.SetTheme(th)
	m.menu.SetTheme(th)
	m.updateMenu.SetTheme(th)
	m.shelfEditor.SetTheme(th)
	m.form.SetTheme(th)
	m.rulesEditor.SetTheme(th)
	m.toast.SetTheme(th)
	m.splitDiff.SetTheme(th)
	m.mergeView.SetTheme(th)
	m.files.SetRender(renderFileNode(th, m.pendingCount, m.nodePicked))
	m.clFiles.SetRender(renderFileNode(th, m.pendingCount, m.nodePicked))
	m.changelists.SetRender(renderChangelistGroup(th, m.groupPicked))
	m.savedDiffs.SetRender(renderSavedDiff(th))
	m.rejects.SetRender(renderRejectNode(th))
	m.shelves.SetRender(renderShelfEntry(th))
	m.refreshChrome()
}

// openSettings shows the settings editor as a centered overlay, populating it
// from the current configuration. The directory-diff field is seeded from the
// live runtime state so the form reflects what the user currently sees; the
// hide-untracked field is seeded from the persisted config instead, since its
// keybind is a session-only toggle the editor must not capture.
func (m *Model) openSettings() tea.Cmd {
	m.form.SetFields(settingsFields(m.cfg, m.dirDiff))
	// The hide rules are edited in their own overlay, so the editor works on a
	// draft the form only writes back to the configuration when it is saved.
	m.rulesDraft = append(make([]config.HideRule, 0, len(m.cfg.HideRules)), m.cfg.HideRules...)
	// Remember the active theme so canceling reverts any live Theme-field preview.
	m.themeBefore = m.cfg.Theme
	m.configuring = true
	m.form.Focus()
	m.sizeForm()
	return nil
}

// closeSettings hides the settings editor without saving, reverting any live
// theme preview to the theme active before the editor opened. submitSettings
// re-applies the chosen theme afterward when it persists a change.
func (m *Model) closeSettings() {
	m.previewTheme(m.themeBefore)
	m.configuring = false
	m.rulesDraft = nil
	m.form.Blur()
}

// submitSettings reads the edited fields back into the configuration, applies the
// changes that take effect immediately (the theme palette, the directory-diff
// default, the hide-untracked toggle and the display scope), persists the result,
// and closes the editor. A blank or non-positive log limit is ignored so the
// previous value survives; a failed save is non-fatal and surfaced as a toast.
func (m *Model) submitSettings() tea.Cmd {
	vals := m.form.Values()
	// Field order mirrors settingsFields.
	cfg := m.cfg
	if n, err := strconv.Atoi(strings.TrimSpace(vals[0])); err == nil && n > 0 {
		cfg.LogLimit = n
	}
	cfg.Editor = strings.TrimSpace(vals[1])
	if cfg.Editor == "" {
		cfg.Editor = config.Default().Editor
	}
	cfg.Theme = strings.TrimSpace(vals[2])
	cfg.DirectoryDiff = vals[3] == "true"
	cfg.HideUntracked = vals[4] == "true"
	cfg.SSHKeyPath = strings.TrimSpace(vals[5])
	if cfg.SSHKeyPath == "" {
		cfg.SSHKeyPath = config.Default().SSHKeyPath
	}
	cfg.DisplayFrom = strings.TrimSpace(vals[6])
	cfg.DiffOutputDir = strings.TrimSpace(vals[7])
	cfg.OptimisticUpdates = vals[8] == "true"
	cfg.LiveRefresh = vals[9] == "true"
	cfg.AllowMouse = vals[10] == "true"
	cfg.HideRules = m.rulesDraft

	m.closeSettings()

	themeChanged := cfg.Theme != m.cfg.Theme
	untrackedChanged := cfg.HideUntracked != m.hideUntracked
	rulesChanged := !slices.Equal(cfg.HideRules, m.cfg.HideRules)
	liveChanged := cfg.LiveRefresh != m.liveRefresh
	mouseChanged := cfg.AllowMouse != m.cfg.AllowMouse
	displayChanged := cfg.DisplayFrom != m.cfg.DisplayFrom
	diffDirChanged := cfg.DiffOutputDir != m.cfg.DiffOutputDir
	logLimitChanged := cfg.LogLimit != m.cfg.LogLimit
	m.cfg = cfg
	m.dirDiff = cfg.DirectoryDiff
	m.hideUntracked = cfg.HideUntracked
	m.liveRefresh = cfg.LiveRefresh
	if themeChanged {
		m.previewTheme(cfg.Theme)
	}
	if untrackedChanged {
		m.rebuildFilesViews()
	}
	// Hide rules narrow the Changes tree alone, so only it is rebuilt.
	if rulesChanged {
		m.compileHideRules()
		m.rebuildFileTree()
	}
	// A new display scope re-roots every svn command, so the status and history
	// on screen no longer describe the tree being shown; load them afresh.
	var reload tea.Cmd
	if displayChanged {
		m.retargetDisplay(cfg.DisplayFrom)
		m.session.Purge()
		m.clearDiff()
		m.loading = true
		m.refreshChrome()
		reload = m.beginInitialLoad()
	}
	// A new display scope restarts the poller as part of the reload above; on its
	// own the setting starts or stops it here.
	if liveChanged && !displayChanged {
		if m.liveRefresh {
			reload = tea.Batch(reload, m.startWatch())
		} else {
			m.stopWatch()
		}
	}
	// Turning the mouse on or off asks the terminal to start or stop reporting it,
	// so the setting takes effect without a restart.
	if mouseChanged {
		reload = tea.Batch(reload, mouseReporting(cfg.AllowMouse))
	}
	// A new output directory means a different set of saved diffs to browse; the
	// display scope changes it too, since a blank setting resolves to the root.
	if diffDirChanged || displayChanged {
		m.savedDiffItems, m.savedDiffsErr = nil, nil
		m.savedPath, m.savedText, m.savedErr = "", "", false
		m.rebuildSavedDiffs()
		reload = tea.Batch(reload, m.reloadSavedDiffsIfShown())
	}
	// Rejects are found by walking the source path, so a new display scope is a
	// different tree to search.
	if displayChanged {
		m.clearRejects()
		reload = tea.Batch(reload, m.reloadRejectsIfShown())
	}
	// A new page size puts different revisions on every page, so the anchors
	// reached with the old one no longer address anything.
	if logLimitChanged && !displayChanged {
		m.log.GoTop()
		reload = tea.Batch(reload, m.resetLogPaging())
	}
	if err := config.Save(m.cfg); err != nil {
		m.showToast("couldn't save settings: "+err.Error(), component.LevelWarning)
		return reload
	}
	m.showToast("settings saved", component.LevelSuccess)
	if reload != nil {
		return reload
	}
	m.updateMain()
	if m.source == sourceFiles {
		return m.diffLoadForSelection()
	}
	return nil
}

// themeFieldIndex is the position of the Theme field within settingsFields; the
// settings editor live-previews the palette when the field at this index
// changes, and submitSettings reads the same position back as the theme.
const themeFieldIndex = 2

// hideRulesFieldIndex is the position of the Hide rules field within
// settingsFields. It is an action field: activating it opens the rules editor,
// and its value is the summary shown on the row.
const hideRulesFieldIndex = 11

// openHideRules raises the hide-rules editor over the settings editor, seeded
// with the draft rules. It is reached by activating the form's Hide rules row.
func (m *Model) openHideRules() tea.Cmd {
	m.rulesEditor.SetEntries(hideRuleEntries(m.rulesDraft))
	m.editingRules = true
	m.form.Blur()
	m.rulesEditor.Focus()
	m.sizeHideRules()
	return nil
}

// closeHideRules drops the rules editor without keeping its edits, handing the
// keyboard back to the settings editor beneath it.
func (m *Model) closeHideRules() {
	m.editingRules = false
	m.rulesEditor.Blur()
	m.form.Focus()
}

// submitHideRules keeps the edited rules in the draft the settings editor will
// save, refreshing the summary on its Hide rules row. A pattern that does not
// compile could never hide anything, so it is reported and the editor stays open
// on it rather than saving a rule that does nothing.
func (m *Model) submitHideRules() tea.Cmd {
	rules := hideRulesFrom(m.rulesEditor.Entries())
	for _, r := range rules {
		if _, err := regexp.Compile(r.Pattern); err != nil {
			m.showToast("invalid pattern: "+r.Pattern, component.LevelWarning)
			return nil
		}
	}
	m.rulesDraft = rules
	m.form.SetValue(hideRulesFieldIndex, hideRulesSummary(rules))
	m.closeHideRules()
	return nil
}

// hideRuleEntries adapts the configured rules to the editor's rows.
func hideRuleEntries(rules []config.HideRule) []component.EditEntry {
	entries := make([]component.EditEntry, 0, len(rules))
	for _, r := range rules {
		entries = append(entries, component.EditEntry{Text: r.Pattern, Enabled: r.Enabled})
	}
	return entries
}

// hideRulesFrom adapts the editor's rows back into configured rules.
func hideRulesFrom(entries []component.EditEntry) []config.HideRule {
	rules := make([]config.HideRule, 0, len(entries))
	for _, e := range entries {
		rules = append(rules, config.HideRule{Pattern: e.Text, Enabled: e.Enabled})
	}
	return rules
}

// hideRulesSummary is what the settings editor's Hide rules row displays: how
// many rules there are and how many of them are in force.
func hideRulesSummary(rules []config.HideRule) string {
	if len(rules) == 0 {
		return "none"
	}
	on := 0
	for _, r := range rules {
		if r.Enabled {
			on++
		}
	}
	noun := "rules"
	if len(rules) == 1 {
		noun = "rule"
	}
	return fmt.Sprintf("%d %s · %d on", len(rules), noun, on)
}

// settingsFields builds the settings editor's fields from the configuration, in
// the field order submitSettings relies on. The directory-diff field is seeded
// from the live runtime state (dirDiff) rather than cfg so the form shows what
// the user currently sees. The hide-untracked field, by contrast, is seeded from
// the persisted configuration: its keybind (U) is a session-only view toggle that
// must never reach the saved config, so the editor shows and edits the global
// default independently of any runtime override. Every other field comes straight
// from the persisted configuration.
func settingsFields(cfg config.Config, dirDiff bool) []component.Field {
	return []component.Field{
		{Label: "Log limit", Kind: component.FieldInt, Value: strconv.Itoa(cfg.LogLimit)},
		{Label: "Editor", Kind: component.FieldChoice, Value: cfg.Editor, Options: config.EditorValues()},
		{Label: "Theme", Kind: component.FieldChoice, Value: cfg.Theme, Options: theme.Names()},
		{Label: "Directory diff", Kind: component.FieldBool, Value: strconv.FormatBool(dirDiff)},
		{Label: "Hide untracked", Kind: component.FieldBool, Value: strconv.FormatBool(cfg.HideUntracked)},
		{Label: "SSH key", Kind: component.FieldText, Value: cfg.SSHKeyPath},
		{Label: "Display from", Kind: component.FieldChoice, Value: cfg.DisplayFrom, Options: config.DisplayFromValues()},
		{Label: "Diff output", Kind: component.FieldText, Value: cfg.DiffOutputDir},
		{Label: "Optimistic updates", Kind: component.FieldBool, Value: strconv.FormatBool(cfg.OptimisticUpdates)},
		{Label: "Live refresh", Kind: component.FieldBool, Value: strconv.FormatBool(cfg.LiveRefresh)},
		{Label: "Allow mouse", Kind: component.FieldBool, Value: strconv.FormatBool(cfg.AllowMouse)},
		{Label: "Hide rules", Kind: component.FieldAction, Value: hideRulesSummary(cfg.HideRules)},
	}
}
