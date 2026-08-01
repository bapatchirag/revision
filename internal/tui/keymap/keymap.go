// Package keymap defines the shared key bindings injected into components. Each
// component reads only the bindings relevant to it; the app owns global keys
// such as focus switching and refresh.
package keymap

import (
	"strings"

	"github.com/charmbracelet/bubbles/key"
)

// KeyMap is the full set of bindings the UI understands.
type KeyMap struct {
	Up        key.Binding
	Down      key.Binding
	Left      key.Binding
	Right     key.Binding
	PageUp    key.Binding
	PageDown  key.Binding
	Top       key.Binding
	Bottom    key.Binding
	LineStart key.Binding
	LineEnd   key.Binding

	WordLeft        key.Binding
	WordRight       key.Binding
	DeleteWordLeft  key.Binding
	DeleteWordRight key.Binding

	Enter   key.Binding
	Confirm key.Binding
	Cancel  key.Binding
	Back    key.Binding
	Submit  key.Binding

	FocusNext key.Binding
	FocusPrev key.Binding

	PrevView key.Binding
	NextView key.Binding

	ToggleDirDiff   key.Binding
	ToggleUntracked key.Binding
	ToggleCmdLog    key.Binding

	SaveDiff key.Binding

	SplitDiff key.Binding

	OpenEditor key.Binding

	Filter    key.Binding
	Refresh   key.Binding
	Settings  key.Binding
	ChangeDir key.Binding
	Help      key.Binding
	Quit      key.Binding
}

// Default returns the standard, lazygit-flavored bindings.
func Default() KeyMap {
	return KeyMap{
		Up:        key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
		Down:      key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
		Left:      key.NewBinding(key.WithKeys("left", "h"), key.WithHelp("←/h", "left")),
		Right:     key.NewBinding(key.WithKeys("right", "l"), key.WithHelp("→/l", "right")),
		PageUp:    key.NewBinding(key.WithKeys("pgup", "K"), key.WithHelp("PgUp", "page up")),
		PageDown:  key.NewBinding(key.WithKeys("pgdown", "J"), key.WithHelp("PgDn", "page down")),
		Top:       key.NewBinding(key.WithKeys("g"), key.WithHelp("g", "top")),
		Bottom:    key.NewBinding(key.WithKeys("G"), key.WithHelp("G", "bottom")),
		LineStart: key.NewBinding(key.WithKeys("home", "^"), key.WithHelp("home", "line start")),
		LineEnd:   key.NewBinding(key.WithKeys("end", "$"), key.WithHelp("end", "line end")),
		// Word-wise editing inside text inputs. Each action lists every encoding a
		// common macOS or Linux terminal sends for it: the modifier form (alt+←),
		// the meta-prefixed form macOS Terminal sends for option+← (alt+b), and the
		// ctrl+arrow / ctrl+w forms used across Linux terminals and readline.
		WordLeft:        key.NewBinding(key.WithKeys("alt+left", "ctrl+left", "alt+b"), key.WithHelp("alt+←", "word left")),
		WordRight:       key.NewBinding(key.WithKeys("alt+right", "ctrl+right", "alt+f"), key.WithHelp("alt+→", "word right")),
		DeleteWordLeft:  key.NewBinding(key.WithKeys("alt+backspace", "ctrl+w"), key.WithHelp("alt+⌫", "delete word left")),
		DeleteWordRight: key.NewBinding(key.WithKeys("alt+delete", "alt+d"), key.WithHelp("alt+⌦", "delete word right")),
		Enter:           key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "select")),
		Confirm:         key.NewBinding(key.WithKeys("enter", "y"), key.WithHelp("enter", "confirm")),
		Cancel:          key.NewBinding(key.WithKeys("esc", "n"), key.WithHelp("esc", "cancel")),
		Back:            key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
		Submit:          key.NewBinding(key.WithKeys("ctrl+s"), key.WithHelp("ctrl+s", "submit")),
		FocusNext:       key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "next panel")),
		FocusPrev:       key.NewBinding(key.WithKeys("shift+tab"), key.WithHelp("shift+tab", "prev panel")),
		PrevView:        key.NewBinding(key.WithKeys("["), key.WithHelp("[", "prev view")),
		NextView:        key.NewBinding(key.WithKeys("]"), key.WithHelp("]", "next view")),
		ToggleDirDiff:   key.NewBinding(key.WithKeys("D"), key.WithHelp("D", "toggle dir diff")),
		ToggleUntracked: key.NewBinding(key.WithKeys("U"), key.WithHelp("U", "toggle untracked")),
		ToggleCmdLog:    key.NewBinding(key.WithKeys("x"), key.WithHelp("x", "toggle command log")),
		SaveDiff:        key.NewBinding(key.WithKeys("w"), key.WithHelp("w", "save diff")),
		SplitDiff:       key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "side-by-side diff")),
		OpenEditor:      key.NewBinding(key.WithKeys("e"), key.WithHelp("e", "open in editor")),
		Filter:          key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "filter")),
		Refresh:         key.NewBinding(key.WithKeys("R"), key.WithHelp("R", "refresh")),
		Settings:        key.NewBinding(key.WithKeys("S"), key.WithHelp("S", "settings")),
		ChangeDir:       key.NewBinding(key.WithKeys("P"), key.WithHelp("P", "change source path")),
		Help:            key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
		Quit:            key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
	}
}

// Section is a titled group of bindings in the help reference.
type Section struct {
	Title    string
	Bindings []Binding
}

// Binding is one row of the help reference: the keys that trigger an action,
// where they apply, and prose describing what they do.
type Binding struct {
	Action      string
	Keys        []string
	Context     string
	Description string

	// sep joins Keys in the overlay's key column; empty means " / ".
	sep string
}

// KeyHint renders the binding's keys as the "?" overlay's key column shows them.
func (b Binding) KeyHint() string {
	sep := b.sep
	if sep == "" {
		sep = " / "
	}
	return strings.Join(b.Keys, sep)
}

// HelpSections is the keybindings reference. It is the single table behind both
// the in-app "?" overlay and the website's keybindings page, so the two cannot
// diverge. Action and Keys drive the overlay; Context and Description are for
// the website, which has room for a sentence.
func HelpSections() []Section {
	return []Section{
		{Title: "Changes", Bindings: []Binding{
			{
				Action: "Stage / unstage", Keys: []string{"space"}, Context: "Files",
				Description: "Stage or unstage the selected file, or every change under the selected directory. An untracked file is `svn add`ed first.",
			},
			{
				Action: "Assign changelist", Keys: []string{"n"}, Context: "Files",
				Description: "Assign the staged set — or just the selected file when nothing is staged — to a named changelist.",
			},
			{
				Action: "Commit staged / list", Keys: []string{"c"}, Context: "Files",
				Description: "Commit the staged files, or the selected changelist.",
			},
			{
				Action: "Expand changelist", Keys: []string{"enter"}, Context: "Files",
				Description: "Expand or collapse the selected directory, or drill into a changelist.",
			},
			{
				Action: "Revert / delete file", Keys: []string{"r", "d"}, Context: "Files",
				Description: "Revert or delete the selected file, or everything under the selected directory. Both ask for confirmation.",
			},
		}},
		{Title: "Working copy", Bindings: []Binding{
			{
				Action: "Update working copy", Keys: []string{"u"}, Context: "Global",
				Description: "Update the working copy to the latest revision.",
			},
			{
				Action: "Update to rev (Log)", Keys: []string{"space"}, Context: "Log",
				Description: "Update the working copy to the selected revision.",
			},
			{
				Action: "Open file in editor", Keys: []string{"e"}, Context: "Files",
				Description: "Open the highlighted file in the configured editor.",
			},
			{
				Action: "Change source path", Keys: []string{"P"}, Context: "Global",
				Description: "Re-scope revision to another directory inside the working copy, browsing to it from a prompt that opens on the directory it is reading now. The working copy's root is fixed, and the new source lasts for the session only — it is never saved.",
			},
			{
				Action: "Refresh / settings", Keys: []string{"R", "S"}, Context: "Global",
				Description: "Refresh status and history, or open the settings editor.",
			},
		}},
		{Title: "Navigation", Bindings: []Binding{
			{
				Action: "Jump to panel", Keys: []string{"1", "2", "3", "4", "0"}, sep: " ", Context: "Global",
				Description: "Focus the Status, Files, Log, Command Log or Main panel.",
			},
			{
				Action: "Cycle panels", Keys: []string{"tab", "shift+tab"}, Context: "Global",
				Description: "Move focus to the next or previous panel.",
			},
			{
				Action: "Move up / down", Keys: []string{"k", "j"}, Context: "Any panel",
				Description: "Move the selection up or down. ↑ and ↓ work too.",
			},
			{
				Action: "Jump top / bottom", Keys: []string{"g", "G"}, Context: "Any panel",
				Description: "Jump to the first or last row.",
			},
			{
				Action: "Next / prev history page", Keys: []string{"n", "p"}, Context: "Log",
				Description: "Load the next or previous page of revision history. A page holds `logLimit` revisions.",
			},
			{
				Action: "Scroll main up / down", Keys: []string{"K", "J"}, Context: "Main",
				Description: "Scroll the Main panel up or down a page.",
			},
			{
				Action: "Scroll main l / r", Keys: []string{"h", "l"}, Context: "Main",
				Description: "Scroll the focused panel one column left or right. ← and → work too.",
			},
			{
				Action: "Line start / end", Keys: []string{"home", "end"}, Context: "Any panel",
				Description: "Jump to the start or end of the line. ^ and $ work too.",
			},
			{
				Action: "Filter panel", Keys: []string{"/"}, Context: "Any panel",
				Description: "Filter the Files or Log panel, or search within Main or Status.",
			},
		}},
		{Title: "View", Bindings: []Binding{
			{
				Action: "Switch file view", Keys: []string{"[", "]"}, Context: "Files",
				Description: "Turn the Files panel between the Changes, Changelists and Diffs views.",
			},
			{
				Action: "Side-by-side / save", Keys: []string{"s", "w"}, Context: "Main",
				Description: "Open the diff on screen side by side in an overlay, or save it to a file in `diffOutputDir`.",
			},
			{
				Action: "Dir diff / untracked", Keys: []string{"D", "U"}, Context: "Files",
				Description: "Toggle the directory-level diff for the highlighted directory, or toggle hiding untracked files.",
			},
			{
				Action: "Toggle command log", Keys: []string{"x"}, Context: "Global",
				Description: "Show or hide the Command Log panel.",
			},
		}},
		{Title: "General", Bindings: []Binding{
			{
				Action: "Toggle help", Keys: []string{"?"}, Context: "Global",
				Description: "Open or close this keybindings reference.",
			},
			{
				Action: "Quit", Keys: []string{"q"}, Context: "Global",
				Description: "Quit revision. ctrl+c works too.",
			},
		}},
	}
}
