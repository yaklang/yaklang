package xml2

import (
	"bytes"
	"encoding/xml"
	"github.com/yaklang/yaklang/common/log"
	"io"
	"strings"
)

type XMLConfig struct {
	onStartElement func(xml.StartElement)
	onEndElement   func(xml.EndElement)
	onCharData     func(xml.CharData, int64)
	onComment      func(xml.Comment)
	onProcInst     func(xml.ProcInst)
	onDirective    func(xml.Directive) bool
	currentOffset  int64
}

type XMLConfigHandler func(*XMLConfig)

func WithStartElementHandler(handler func(xml.StartElement)) XMLConfigHandler {
	return func(c *XMLConfig) {
		c.onStartElement = handler
	}
}

func WithEndElementHandler(handler func(xml.EndElement)) XMLConfigHandler {
	return func(c *XMLConfig) {
		c.onEndElement = handler
	}
}

func WithCharDataHandler(handler func(xml.CharData, int64)) XMLConfigHandler {
	return func(c *XMLConfig) {
		c.onCharData = handler
	}
}

func WithCommentHandler(handler func(xml.Comment)) XMLConfigHandler {
	return func(c *XMLConfig) {
		c.onComment = handler
	}
}

func WithProcInstHandler(handler func(xml.ProcInst)) XMLConfigHandler {
	return func(c *XMLConfig) {
		c.onProcInst = handler
	}
}

func WithDirectiveHandler(handler func(xml.Directive) bool) XMLConfigHandler {
	return func(c *XMLConfig) {
		c.onDirective = handler
	}
}

func Handle(value string, opts ...XMLConfigHandler) {
	c := &XMLConfig{}
	for _, opt := range opts {
		opt(c)
	}

	decoder := xml.NewDecoder(strings.NewReader(value))

	doctype := false

	for {
		t, err := decoder.Token()
		if err != nil {
			if err != io.EOF {
				log.Errorf("error: %v", err)
			}
			break
		}
		if !doctype {
			switch se := t.(type) {
			case xml.Directive:
				se = bytes.TrimSpace(se)
				if strings.HasPrefix(string(se), `DOCTYPE`) || strings.HasPrefix(string(se), `doctype`) {
					doctype = true
					if c.onDirective != nil {
						if !c.onDirective(se) {
							return
						}
					}
				}
			}
			continue
		}

		switch se := t.(type) {
		case xml.StartElement:
			if c.onStartElement != nil {
				c.onStartElement(se)
			}
		case xml.EndElement:
			if c.onEndElement != nil {
				c.onEndElement(se)
			}
		case xml.CharData:
			if c.onCharData != nil {
				c.onCharData(se, c.currentOffset)
			}
		case xml.Comment:
			if c.onComment != nil {
				c.onComment(se)
			}
		case xml.ProcInst:
			if c.onProcInst != nil {
				c.onProcInst(se)
			}
		default:
			log.Infof("unknown: %T", se)
		}
		c.currentOffset = decoder.InputOffset()
	}
}
