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

// Log returns up to limit recent revisions reported by `svn log`. A limit of
// zero or less applies svn's own default. Changed paths are included (--verbose).
func (c *Client) Log(ctx context.Context, limit int) ([]LogEntry, error) {
	args := []string{"log", "--xml", "--verbose"}
	if limit > 0 {
		args = append(args, "--limit", strconv.Itoa(limit))
	}
	out, err := c.run(ctx, args...)
	if err != nil {
		return nil, err
	}
	return parseLog(out)
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
