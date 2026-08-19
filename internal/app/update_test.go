package app

import (
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/bapatchirag/revision/internal/svn"
	uimsg "github.com/bapatchirag/revision/internal/tui/msg"
)

// probeMsgs is one value of every message Update dispatches. A message carrying
// an error is probed with one so its handler takes the short reporting path
// instead of acting on a zero value.
func probeMsgs() []tea.Msg {
	err := errors.New("probe")
	return []tea.Msg{
		// systemEvent
		tea.WindowSizeMsg{Width: 80, Height: 24},
		errMsg{err: err},
		workingCopyChangedMsg{},
		updateAvailableMsg{},
		startupNoticeMsg{},
		sshCheckedMsg{err: err},
		sshAddedMsg{err: err},
		// probeSourceCmd is the only producer and always sets client, error or not.
		sourceChangedMsg{client: &svn.Client{}, err: err},
		reposFoundMsg{},

		// loadEvent
		statusLoadedMsg{},
		logLoadedMsg{err: err},
		headLoadedMsg{err: err},
		revisionPendingMsg{},
		revisionDetailMsg{err: err},
		revDiffLoadedMsg{err: err},
		diffPendingMsg{},
		diffLoadedMsg{err: err},
		savedDiffsLoadedMsg{err: err},
		savedDiffReadMsg{err: err},
		rejectsLoadedMsg{err: err},
		rejectReadMsg{err: err},
		shelvesLoadedMsg{err: err},
		shelfReadMsg{err: err},
		mergeLoadedMsg{err: err},

		// mutationEvent
		stagedMsg{err: err},
		committedMsg{err: err},
		revertedMsg{err: err},
		deletedMsg{err: err},
		updatedMsg{err: err},
		diffSavedMsg{err: err},
		savedDiffDeletedMsg{err: err},
		rejectDeletedMsg{err: err},
		patchAppliedMsg{err: err},
		mergeWrittenMsg{err: err},
		editedMsg{err: err},

		// uiEvent
		uimsg.SelectedMsg{},
		uimsg.ActivatedMsg{},
		uimsg.ViewSelectedMsg{},
		uimsg.SubViewPoppedMsg{},
		uimsg.SubmitMsg{},
		uimsg.ConfirmMsg{},
		uimsg.DismissMsg{},
	}
}

// handlerNames are Update's dispatchers, in the order it offers a message to
// them. dispatchers returns them bound to m in the same order.
var handlerNames = []string{"systemEvent", "loadEvent", "mutationEvent", "uiEvent"}

func dispatchers(m *Model) []func(tea.Msg) (tea.Cmd, bool) {
	return []func(tea.Msg) (tea.Cmd, bool){m.systemEvent, m.loadEvent, m.mutationEvent, m.uiEvent}
}

// TestEveryMessageIsClaimedOnce is the invariant that keeps Update's four
// handlers a partition rather than a race: a message claimed twice would have
// its second handler silently skipped, and one claimed by none would fall
// through to the focused panel.
func TestEveryMessageIsClaimedOnce(t *testing.T) {
	for _, msg := range probeMsgs() {
		t.Run(fmt.Sprintf("%T", msg), func(t *testing.T) {
			var claimed []string
			for i, name := range handlerNames {
				// A fresh model per handler, so one handler's side effects cannot
				// change what the next one does with the same message.
				m := sizedModel(t)
				if _, ok := dispatchers(m)[i](msg); ok {
					claimed = append(claimed, name)
				}
			}
			if len(claimed) != 1 {
				t.Fatalf("claimed by %v, want exactly one handler", claimed)
			}
		})
	}
}

// TestKeysAreLeftToRouteKey holds the other half of the partition: keys are not
// a message any dispatcher owns, so routeKey is reached with the overlay state
// the message handlers left behind.
func TestKeysAreLeftToRouteKey(t *testing.T) {
	for i, name := range handlerNames {
		m := sizedModel(t)
		if _, ok := dispatchers(m)[i](tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}}); ok {
			t.Errorf("%s claimed a key; keys belong to routeKey", name)
		}
	}
}

// TestMouseIsLeftToRouteMouse is the same line for the pointer: a dispatcher
// claiming a mouse message would leave routeMouse dead.
func TestMouseIsLeftToRouteMouse(t *testing.T) {
	for i, name := range handlerNames {
		m := sizedModel(t)
		if _, ok := dispatchers(m)[i](click(0, 0)); ok {
			t.Errorf("%s claimed a mouse event; the mouse belongs to routeMouse", name)
		}
	}
}

// TestProbeCoversEveryMessageType stops a message type added later from
// escaping the partition check above unnoticed.
func TestProbeCoversEveryMessageType(t *testing.T) {
	probed := map[string]bool{}
	for _, msg := range probeMsgs() {
		probed[strings.TrimPrefix(fmt.Sprintf("%T", msg), "app.")] = true
	}

	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read package dir: %v", err)
	}
	fset := token.NewFileSet()
	declared := 0
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, filepath.Join(".", name), nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		for _, d := range f.Decls {
			gd, ok := d.(*ast.GenDecl)
			if !ok || gd.Tok != token.TYPE {
				continue
			}
			for _, spec := range gd.Specs {
				ts, ok := spec.(*ast.TypeSpec)
				if !ok || !strings.HasSuffix(ts.Name.Name, "Msg") {
					continue
				}
				declared++
				if !probed[ts.Name.Name] {
					t.Errorf("%s is declared in %s but not in probeMsgs", ts.Name.Name, name)
				}
			}
		}
	}
	if declared == 0 {
		t.Fatal("no message types found; the source scan is not looking where it thinks")
	}
}
