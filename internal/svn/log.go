package svn

import (
	"context"
	"encoding/xml"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// logXML mirrors the structure of `svn log --xml`.
type logXML struct {
	XMLName xml.Name      `xml:"log"`
	Entries []logEntryXML `xml:"logentry"`
}

type logEntryXML struct {
	Revision string `xml:"revision,attr"`
	Author   string `xml:"author"`
	Date     string `xml:"date"`
	Msg      string `xml:"msg"`
	Paths    struct {
		Paths []struct {
			Action string `xml:"action,attr"`
			Path   string `xml:",chardata"`
		} `xml:"path"`
	} `xml:"paths"`
}

// LogPage returns one page of at most limit revisions reported by `svn log`,
// newest first, and reports whether a further page exists. An empty anchor
// starts at the newest revision; otherwise the page starts at the revision
// following anchor, which must be a revision from the preceding page. A limit of
// zero or less applies svn's own default and never reports a further page.
// Changed paths are included (--verbose).
//
// It targets the working-copy root pegged at HEAD (".@HEAD") rather than the
// default BASE. After committing a single path the working copy is left at a
// mixed revision — the containing directory stays at the old revision — so a
// default `svn log` would stop at BASE and omit the just-created revision until
// the next `svn update`. Pegging at HEAD logs from the repository head instead,
// and stays safe (empty, not an error) on a freshly created repository.
func (c *Client) LogPage(ctx context.Context, anchor string, limit int) ([]LogEntry, bool, error) {
	out, err := c.run(ctx, logPageArgs(anchor, limit)...)
	if err != nil {
		return nil, false, err
	}
	entries, err := parseLog(out)
	if err != nil {
		return nil, false, err
	}
	page, more := logPageFrom(entries, anchor, limit)
	return page, more, nil
}

// logPageArgs builds the `svn log` invocation for one page. Revision numbers are
// not contiguous for a given path, so a page cannot be addressed by arithmetic;
// it is anchored on a revision from the preceding page instead. The range is
// deliberately over-fetched — see logPageFrom for what the extra rows are for.
func logPageArgs(anchor string, limit int) []string {
	args := []string{"log", "--xml", "--verbose"}
	if anchor != "" {
		args = append(args, "-r", anchor+":1")
	}
	if limit > 0 {
		args = append(args, "--limit", strconv.Itoa(limit+logPageExtra(anchor)))
	}
	return append(args, ".@HEAD")
}

// logPageExtra is how many rows beyond the page size to request: one to detect a
// further page, plus one for the anchor revision itself when anchored, since
// `-r <anchor>:1` is inclusive and repeats it as the first entry.
func logPageExtra(anchor string) int {
	if anchor == "" {
		return 1
	}
	return 2
}

// logPageFrom trims an over-fetched result down to one page, dropping the
// repeated anchor revision and the surplus row that only signals a further page.
func logPageFrom(entries []LogEntry, anchor string, limit int) ([]LogEntry, bool) {
	if anchor != "" && len(entries) > 0 && entries[0].Revision == anchor {
		entries = entries[1:]
	}
	if limit > 0 && len(entries) > limit {
		return entries[:limit], true
	}
	return entries, false
}

func parseLog(data []byte) ([]LogEntry, error) {
	var doc logXML
	if err := xml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parse svn log xml: %w", err)
	}
	entries := make([]LogEntry, 0, len(doc.Entries))
	for _, e := range doc.Entries {
		entries = append(entries, logEntryFrom(e))
	}
	return entries, nil
}

func logEntryFrom(e logEntryXML) LogEntry {
	entry := LogEntry{
		Revision: e.Revision,
		Author:   e.Author,
		Message:  strings.TrimSpace(e.Msg),
	}
	if t, err := time.Parse(time.RFC3339, e.Date); err == nil {
		entry.Date = t
	}
	for _, p := range e.Paths.Paths {
		entry.Paths = append(entry.Paths, ChangedPath{Action: p.Action, Path: strings.TrimSpace(p.Path)})
	}
	return entry
}
