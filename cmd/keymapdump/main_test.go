package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/bapatchirag/revision/internal/tui/keymap"
)

func TestDumpEmitsEveryBinding(t *testing.T) {
	var buf bytes.Buffer
	if err := dump(&buf); err != nil {
		t.Fatalf("dump: %v", err)
	}

	var rows []row
	if err := json.Unmarshal(buf.Bytes(), &rows); err != nil {
		t.Fatalf("dump did not produce valid JSON: %v", err)
	}

	want := 0
	for _, s := range keymap.HelpSections() {
		want += len(s.Bindings)
	}
	if len(rows) != want {
		t.Errorf("dumped %d rows, want one per binding (%d)", len(rows), want)
	}
	for i, r := range rows {
		if r.Section == "" || r.Action == "" || len(r.Keys) == 0 || r.Context == "" || r.Description == "" {
			t.Errorf("row %d is missing a field: %+v", i, r)
		}
	}
	if got := buf.Bytes(); got[len(got)-1] != '\n' {
		t.Error("output should end with a newline")
	}
}

// TestSiteDataIsUpToDate catches drift between the Go keymap and the JSON the
// website renders from. Regenerate with `make site-data`.
func TestSiteDataIsUpToDate(t *testing.T) {
	path := filepath.Join("..", "..", "site", "src", "data", "keybindings.json")
	want, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("site data not present: %v", err)
	}

	var buf bytes.Buffer
	if err := dump(&buf); err != nil {
		t.Fatalf("dump: %v", err)
	}
	if !bytes.Equal(bytes.TrimSpace(buf.Bytes()), bytes.TrimSpace(want)) {
		t.Error("site/src/data/keybindings.json is stale; run `make site-data`")
	}
}
