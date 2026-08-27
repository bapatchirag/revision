package svn

import (
	"context"
	"encoding/xml"
	"fmt"
	"sort"
)

// statusXML mirrors the structure of `svn status --xml`.
type statusXML struct {
	XMLName xml.Name `xml:"status"`
	Targets []struct {
		Path    string           `xml:"path,attr"`
		Entries []statusEntryXML `xml:"entry"`
	} `xml:"target"`
	Changelists []struct {
		Name    string           `xml:"name,attr"`
		Entries []statusEntryXML `xml:"entry"`
	} `xml:"changelist"`
}

type statusEntryXML struct {
	Path     string `xml:"path,attr"`
	WCStatus struct {
		Item      string `xml:"item,attr"`
		Props     string `xml:"props,attr"`
		Revision  string `xml:"revision,attr"`
		Copied    string `xml:"copied,attr"`
		MovedFrom string `xml:"moved-from,attr"`
		MovedTo   string `xml:"moved-to,attr"`
	} `xml:"wc-status"`
}

// Status returns the working-copy status entries reported by `svn status`.
func (c *Client) Status(ctx context.Context) ([]StatusItem, error) {
	out, err := c.run(ctx, "status", "--xml")
	if err != nil {
		return nil, err
	}
	return parseStatus(out)
}

// StatusPaths returns the status entries for the given paths only, so a caller
// that already knows what it changed can re-read those instead of paying for a
// crawl of the whole working copy.
//
// The read recurses, as `svn status` does by default: naming a directory covers
// everything beneath it. A path svn reports nothing for — it came clean, or it
// is no longer there — yields no entry, which is how a row is learned to be
// gone; svn says so with a warning on stderr and still exits zero.
//
// Naming no path returns nothing rather than running the command: `svn status`
// with no target reads the whole working copy, which is the opposite of what a
// caller asking for a few paths wants. The paths are passed as arguments —
// `svn status` is one of the subcommands that does not take `--targets`
// (verified against svn 1.14.5) — so a caller with a large set is better served
// by Status.
func (c *Client) StatusPaths(ctx context.Context, paths []string) ([]StatusItem, error) {
	if len(paths) == 0 {
		return nil, nil
	}
	out, err := c.run(ctx, append([]string{"status", "--xml"}, paths...)...)
	if err != nil {
		return nil, err
	}
	return parseStatus(out)
}

func parseStatus(data []byte) ([]StatusItem, error) {
	var doc statusXML
	if err := xml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parse svn status xml: %w", err)
	}

	var items []StatusItem
	for _, t := range doc.Targets {
		for _, e := range t.Entries {
			items = append(items, statusItemFrom(e, ""))
		}
	}
	for _, cl := range doc.Changelists {
		for _, e := range cl.Entries {
			items = append(items, statusItemFrom(e, cl.Name))
		}
	}

	sort.SliceStable(items, func(i, j int) bool { return items[i].Path < items[j].Path })
	return items, nil
}

func statusItemFrom(e statusEntryXML, changelist string) StatusItem {
	return StatusItem{
		Path:       e.Path,
		State:      mapState(e.WCStatus.Item),
		PropState:  mapState(e.WCStatus.Props),
		Revision:   e.WCStatus.Revision,
		Changelist: changelist,
		// svn reports the move attributes from 1.9 on; an older client simply
		// omits them and every move reads as an unrelated add and delete.
		Copied:    e.WCStatus.Copied == "true",
		MovedFrom: e.WCStatus.MovedFrom,
		MovedTo:   e.WCStatus.MovedTo,
	}
}
