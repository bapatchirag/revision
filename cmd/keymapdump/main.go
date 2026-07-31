// Command keymapdump prints keymap.HelpSections as a flat JSON array, one row
// per binding, so the website's keybindings page renders the same table as the
// in-app "?" overlay. Output is deterministic: regenerate with `make site-data`.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/bapatchirag/revision/internal/tui/keymap"
)

// row is the wire format consumed by site/src/data/keybindings.json.
type row struct {
	Section     string   `json:"section"`
	Action      string   `json:"action"`
	Keys        []string `json:"keys"`
	Context     string   `json:"context"`
	Description string   `json:"description"`
}

func main() {
	if err := dump(os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "keymapdump:", err)
		os.Exit(1)
	}
}

func dump(w io.Writer) error {
	var rows []row
	for _, s := range keymap.HelpSections() {
		for _, b := range s.Bindings {
			rows = append(rows, row{
				Section:     s.Title,
				Action:      b.Action,
				Keys:        b.Keys,
				Context:     b.Context,
				Description: b.Description,
			})
		}
	}

	out, err := json.MarshalIndent(rows, "", "  ")
	if err != nil {
		return err
	}
	_, err = w.Write(append(out, '\n'))
	return err
}
