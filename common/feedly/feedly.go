package feedly

import (
	"github.com/gilliek/go-opml/opml"
	"github.com/yaklang/yaklang/common/bindata"
)

func viewOutlines(ols []opml.Outline, f func(outline opml.Outline)) {
	for _, ol := range ols {
		if ol.XMLURL != "" {
			f(ol)
		}

		if len(ol.Outlines) > 0 {
			viewOutlines(ol.Outlines, f)
		}
	}
}

func LoadOpmlRawToOutlines(raw []byte) ([]opml.Outline, error) {
	data, err := opml.NewOPML(raw)
	if err != nil {
		return nil, err
	}

	var outls []opml.Outline
	viewOutlines(data.Outlines(), func(outline opml.Outline) {
		outls = append(outls, outline)
	})
	return outls, nil
}

func LoadOutlinesFromBindata() ([]opml.Outline, error) {
	raw, err := bindata.Asset("data/rss/feedly.opml")
	if err != nil {
		return nil, err
	}

	var outls []opml.Outline
	var r []opml.Outline
	r, _ = LoadOpmlRawToOutlines(raw)
	outls = append(outls, r...)

	raw, err = bindata.Asset("data/rss/cyber_security_rss.opml")
	if err != nil {
		return nil, err
	}

	r, _ = LoadOpmlRawToOutlines(raw)
	outls = append(outls, r...)

	return outls, nil
}
