package svn

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
)

// infoXML mirrors the subset of `svn info --xml` we consume.
type infoXML struct {
	XMLName xml.Name `xml:"info"`
	Entries []struct {
		Path       string `xml:"path,attr"`
		Revision   string `xml:"revision,attr"`
		URL        string `xml:"url"`
		Repository struct {
			Root string `xml:"root"`
		} `xml:"repository"`
		WCInfo struct {
			WCRootAbspath string `xml:"wcroot-abspath"`
		} `xml:"wc-info"`
	} `xml:"entry"`
}

// Info returns information about the working copy at the client's directory.
// It returns an error if the directory is not a Subversion working copy.
func (c *Client) Info(ctx context.Context) (*Info, error) {
	out, err := c.run(ctx, "info", "--xml")
	if err != nil {
		return nil, err
	}
	return parseInfo(out)
}

func parseInfo(data []byte) (*Info, error) {
	var doc infoXML
	if err := xml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parse svn info xml: %w", err)
	}
	if len(doc.Entries) == 0 {
		return nil, errors.New("svn info returned no entries")
	}
	e := doc.Entries[0]
	return &Info{
		Path:            e.Path,
		WorkingCopyRoot: e.WCInfo.WCRootAbspath,
		URL:             e.URL,
		RepositoryRoot:  e.Repository.Root,
		Revision:        e.Revision,
	}, nil
}
