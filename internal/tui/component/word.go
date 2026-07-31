package component

import "unicode"

// Word-wise motion and deletion shared by the text-entry components (SearchBar,
// Prompt, Form and TextArea) so option/alt+arrow and option/alt+delete behave
// the same everywhere. A word is a run of letters, digits and underscores, which
// is what macOS text fields and readline both use: whitespace *and* punctuation
// break words, so "internal/tui/word.go" is four words, not one.

// isWordRune reports whether r belongs to a word rather than separating two.
func isWordRune(r rune) bool {
	return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r)
}

// wordStart returns the column at the start of the word before col: the
// separators immediately behind the cursor are skipped, then the word runes
// behind those.
func wordStart(runes []rune, col int) int {
	col = clampCol(runes, col)
	for col > 0 && !isWordRune(runes[col-1]) {
		col--
	}
	for col > 0 && isWordRune(runes[col-1]) {
		col--
	}
	return col
}

// wordEnd returns the column at the end of the word after col: the separators
// ahead of the cursor are skipped, then the word runes after those.
func wordEnd(runes []rune, col int) int {
	col = clampCol(runes, col)
	for col < len(runes) && !isWordRune(runes[col]) {
		col++
	}
	for col < len(runes) && isWordRune(runes[col]) {
		col++
	}
	return col
}

// cutWordLeft deletes the word before col, returning the new text and cursor.
func cutWordLeft(runes []rune, col int) ([]rune, int) {
	col = clampCol(runes, col)
	start := wordStart(runes, col)
	if start == col {
		return runes, col
	}
	next := make([]rune, 0, len(runes)-(col-start))
	next = append(next, runes[:start]...)
	next = append(next, runes[col:]...)
	return next, start
}

// cutWordRight deletes the word after col, returning the new text. The cursor
// does not move, so it needs no separate return.
func cutWordRight(runes []rune, col int) []rune {
	col = clampCol(runes, col)
	end := wordEnd(runes, col)
	if end == col {
		return runes
	}
	next := make([]rune, 0, len(runes)-(end-col))
	next = append(next, runes[:col]...)
	next = append(next, runes[end:]...)
	return next
}

func clampCol(runes []rune, col int) int {
	if col < 0 {
		return 0
	}
	if col > len(runes) {
		return len(runes)
	}
	return col
}
