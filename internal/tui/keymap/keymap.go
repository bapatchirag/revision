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

	ToggleDirDiff     key.Binding
	ToggleUntracked   key.Binding
	ToggleCmdLog      key.Binding
	ToggleLiveRefresh key.Binding

	SaveDiff key.Binding

	SplitDiff key.Binding

	OpenEditor key.Binding

	Filter     key.Binding
	Refresh    key.Binding
	Settings   key.Binding
	ChangeDir  key.Binding
	SwitchRepo key.Binding
	Help       key.Binding
	Quit       key.Binding
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
		WordLeft:          key.NewBinding(key.WithKeys("alt+left", "ctrl+left", "alt+b"), key.WithHelp("alt+←", "word left")),
		WordRight:         key.NewBinding(key.WithKeys("alt+right", "ctrl+right", "alt+f"), key.WithHelp("alt+→", "word right")),
		DeleteWordLeft:    key.NewBinding(key.WithKeys("alt+backspace", "ctrl+w"), key.WithHelp("alt+⌫", "delete word left")),
		DeleteWordRight:   key.NewBinding(key.WithKeys("alt+delete", "alt+d"), key.WithHelp("alt+⌦", "delete word right")),
		Enter:             key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "select")),
		Confirm:           key.NewBinding(key.WithKeys("enter", "y"), key.WithHelp("enter", "confirm")),
		Cancel:            key.NewBinding(key.WithKeys("esc", "n"), key.WithHelp("esc", "cancel")),
		Back:              key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
		Submit:            key.NewBinding(key.WithKeys("ctrl+s"), key.WithHelp("ctrl+s", "submit")),
		FocusNext:         key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "next panel")),
		FocusPrev:         key.NewBinding(key.WithKeys("shift+tab"), key.WithHelp("shift+tab", "prev panel")),
		PrevView:          key.NewBinding(key.WithKeys("["), key.WithHelp("[", "prev view")),
		NextView:          key.NewBinding(key.WithKeys("]"), key.WithHelp("]", "next view")),
		ToggleDirDiff:     key.NewBinding(key.WithKeys("D"), key.WithHelp("D", "toggle dir diff")),
		ToggleUntracked:   key.NewBinding(key.WithKeys("U"), key.WithHelp("U", "toggle untracked")),
		ToggleCmdLog:      key.NewBinding(key.WithKeys("x"), key.WithHelp("x", "toggle command log")),
		ToggleLiveRefresh: key.NewBinding(key.WithKeys("L"), key.WithHelp("L", "toggle live refresh")),
		SaveDiff:          key.NewBinding(key.WithKeys("w"), key.WithHelp("w", "save diff")),
		SplitDiff:         key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "side-by-side diff")),
		OpenEditor:        key.NewBinding(key.WithKeys("e"), key.WithHelp("e", "open in editor")),
		Filter:            key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "filter")),
		Refresh:           key.NewBinding(key.WithKeys("R"), key.WithHelp("R", "refresh")),
		Settings:          key.NewBinding(key.WithKeys("S"), key.WithHelp("S", "settings")),
		ChangeDir:         key.NewBinding(key.WithKeys("P"), key.WithHelp("P", "change source path")),
		SwitchRepo:        key.NewBinding(key.WithKeys("W"), key.WithHelp("W", "switch repository")),
		Help:              key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
		Quit:              key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
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
				Action: "Add to version control", Keys: []string{"a"}, Context: "Files",
				Description: "Put the selection under version control with `svn add`. On a file row it adds that file, on a directory row everything untracked beneath it, and an untracked directory is added with its whole contents, since that is how `svn add` reads one. Anything already versioned, and anything ignored, is left alone. The rows restyle on the keypress and go back if svn refuses them; `r` undoes an add, leaving the file on disk and untracked again.",
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
				Description: "Revert or delete the selected file, or everything under the selected directory. In the Diffs view `d` deletes the highlighted patch file from `diffOutputDir` instead. All ask for confirmation.",
			},
			{
				Action: "Apply saved patch", Keys: []string{"p"}, Context: "Diffs",
				Description: "Apply the patch file highlighted in the Diffs view to the source path, after asking for confirmation. A patch taken from another directory — one whose files are not the ones here — is refused, as is one svn says would land nothing at all. A patch that only partly fits is applied for what it is worth, with the hunks svn could not place written out beside their targets as `.rej` files.",
			},
			{
				Action: "Resolve conflict", Keys: []string{"m"}, Context: "Files",
				Description: "Resolve the selection side by side. Which view you are in decides what it resolves, and the two are never mixed: in the Changes view (or an expanded changelist) it reads the conflict markers in the selected `C` file, and in the Rejects view it reads the selected `.rej` against the file it was written for. A patch that leaves a file both conflicted and with a reject beside it is therefore two pieces of work, one in each view. Each conflict, or each rejected hunk that still fits, is a page: `1` takes the left side, `2` the right, `3` both and `0` clears the choice. Once every page has been decided, `w` writes the file back out and clears what marked it — `svn resolve` for a conflict, removing the reject for a reject. `e` opens the file in your editor to merge it by hand instead.",
			},
		}},
		{Title: "Shelf", Bindings: []Binding{
			{
				Action: "Pick / shelve", Keys: []string{"v", "z"}, Context: "Files",
				Description: "Hold the selection for the next shelve, or take what is held out of the working copy. `v` works on a file, on a directory — everything shelvable beneath it — and on a changelist row, so changes already filed under one are shelved without drilling in; pressed again it lets them go, as does `esc`. A pick is held by path rather than by row, so it survives a reload, a filter and a rebuild. `z` opens a prompt for a name and then takes what is held; with nothing held it means the whole working copy, and asks first.",
			},
			{
				Action: "Apply / pop", Keys: []string{"enter", "p"}, Context: "Shelf",
				Description: "Merge the highlighted entry back into the working copy. `enter` leaves it on the shelf, `p` takes it off — but only when the whole of it went back, since while a hunk is sitting in a `.rej` or an unversioned file could not be placed, the shelf is the only remaining copy of what did not make it. Both ask first, and say so when the entry was shelved at a revision the working copy has since moved off.",
			},
			{
				Action: "Drop / rename", Keys: []string{"d", "n"}, Context: "Shelf",
				Description: "Delete the highlighted entry, after asking — nothing else has a copy of what it holds — or relabel it, from a prompt that opens on the name it has now.",
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
				Description: "Open the highlighted file in the configured editor, positioned on its first changed hunk. Pressed with the Diff panel focused it opens on the line under the cursor instead, so the editor picks up where the eye left off — which is also how a file is picked out of a directory's combined diff. It works from the side-by-side and resolution overlays too, on the file whose page is open.",
			},
			{
				Action: "Change source path", Keys: []string{"P"}, Context: "Global",
				Description: "Re-scope revision to another directory inside the working copy, browsing to it from a prompt that opens on the directory it is reading now. The working copy's root is fixed, and the new source lasts for the session only — it is never saved.",
			},
			{
				Action: "Switch repository", Keys: []string{"W"}, Context: "Global",
				Description: "Move revision to another working copy altogether. The prompt opens at once and the checkouts it finds fill in behind it, so a slow filesystem never holds up the keystroke; typing narrows the list. It looks in the directory revision was started in, in the checkout it is reading, and in that checkout's parents a few levels up, so a sibling of the current tree is offered as readily as something inside it — nearest first. The search stops at each checkout it finds, since everything under one belongs to it, and gives up after a few seconds rather than walking a large tree to the end; anything it did not reach is opened by typing its path in full. A path that is not an SVN working copy is refused and nothing changes. The switch lasts for the session only.",
			},
			{
				Action: "Refresh / settings", Keys: []string{"R", "S"}, Context: "Global",
				Description: "Refresh status and history, or open the settings editor.",
			},
			{
				Action: "Toggle live refresh", Keys: []string{"L"}, Context: "Global",
				Description: "Turn background watching of the working copy on or off. While it is on, a change made outside revision reaches the Files and diff panels on its own. The toggle lasts for the session only; `liveRefresh` sets the default.",
			},
		}},
		{Title: "Navigation", Bindings: []Binding{
			{
				Action: "Jump to panel", Keys: []string{"1", "2", "3", "4", "0"}, sep: " ", Context: "Global",
				Description: "Focus the Status, Files, Log, Shelf or Main panel. The Command Log has no number: `x` shows it and a click focuses it.",
			},
			{
				Action: "Cycle panels", Keys: []string{"tab", "shift+tab"}, Context: "Global",
				Description: "Move focus to the next or previous side panel — Status, Files, Log, Shelf. Main and the Command Log are outside the cycle, so `tab` pressed on either returns to the side panel driving Main.",
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
				Description: "Turn the Files panel between the Changes, Changelists, Diffs and Rejects views.",
			},
			{
				Action: "Side-by-side / save", Keys: []string{"s", "w"}, Context: "Main",
				Description: "Open the diff on screen side by side in an overlay, or save it to a file in `diffOutputDir`. `w` saves a range of history too, down to the single file the drilled-in tree points at; that patch has already been read, so it is written as it stands rather than asked of svn a second time.",
			},
			{
				Action: "Pick revisions", Keys: []string{"v"}, Context: "Log",
				Description: "Pick the selected revision to be diffed, or unpick it. Two can be held at once, and picking a third drops whichever was picked first, so the far end of a comparison can be moved without unpicking it each time. `esc` lets them go. A pick is held by revision rather than by row, so it survives paging and filtering — the two ends need not be on the same page.",
			},
			{
				Action: "Diff picked revisions", Keys: []string{"enter"}, Context: "Log",
				Description: "Diff whatever is picked, and open the files it touched as a tree in place of the revisions. One revision on its own is compared with the one before it, so what that commit changed is what you see. Two are compared with each other as they stand: the diff runs from the older to the newer, which is the state at one against the state at the other rather than the sum of the commits between them — the older revision's own change is already on the left-hand side and is not part of it. The tree reads like the Changes view: a file shows its own patch, a directory everything beneath it, and `enter` folds a directory away. It covers the directory `displayFrom` roots the views at. `esc` comes back out to the revisions, leaving the picks held for another look.",
			},
			{
				Action: "Dir diff / untracked", Keys: []string{"D", "U"}, Context: "Files",
				Description: "Toggle the directory-level diff for the highlighted directory, or toggle hiding untracked files.",
			},
			{
				Action: "Toggle command log", Keys: []string{"x"}, Context: "Global",
				Description: "Show or hide the Command Log panel. It sits outside the panel cycle and has no number, so clicking it is the only way to focus and scroll it.",
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
