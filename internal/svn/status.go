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
		Item     string `xml:"item,attr"`
		Props    string `xml:"props,attr"`
		Revision string `xml:"revision,attr"`
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
	}
}
