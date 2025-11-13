package xml_tools

import (
	"bytes"
	"fmt"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/orderedmap"
	"github.com/yaklang/yaklang/common/utils/yakxml"
	"io"
	"strings"

	"github.com/yaklang/yaklang/common/log"
	"golang.org/x/net/html/charset"
)

func XmlEscape(s []byte) string {
	var w strings.Builder
	yakxml.Escape(&w, s)
	return w.String()
}

type XmlDumpConfig struct {
	escapeHTML bool
}

type XmlDumpOptions func(*XmlDumpConfig)

func WithHTMLEscape(escape bool) XmlDumpOptions {
	return func(c *XmlDumpConfig) {
		c.escapeHTML = escape
	}
}

func NewXmlDumpConfig() *XmlDumpConfig {
	return &XmlDumpConfig{
		escapeHTML: true,
	}
}

func XmlDumps(v interface{}, opts ...XmlDumpOptions) []byte {
	config := NewXmlDumpConfig()
	for _, opt := range opts {
		opt(config)
	}
	var b bytes.Buffer
	var data *orderedmap.OrderedMap
	switch ret := v.(type) {
	case orderedmap.OrderedMap:
		data = &ret
	case *orderedmap.OrderedMap:
		data = ret
	default:
		data = orderedmap.New(utils.InterfaceToGeneralMap(v))
	}
	enc := yakxml.NewEncoderWithEscape(&b, config.escapeHTML)
	enc.Indent("", "  ")
	err := enc.Encode(data)
	if err != nil {
		log.Errorf("xml encode error: %v", err)
	}

	return b.Bytes()
}

func XmlLoadsOmap(v interface{}) (*orderedmap.OrderedMap, error) {
	i := orderedmap.New()
	buf := bytes.NewBufferString(fmt.Sprintf("<root>%s</root>", utils.InterfaceToString(v)))
	decoder := yakxml.NewDecoder(buf)
	decoder.CharsetReader = func(label string, input io.Reader) (io.Reader, error) {
		e, _ := charset.Lookup(label)
		if e != nil {
			return e.NewDecoder().Reader(input), nil
		}
		return input, nil // default to utf-8
	}
	err := decoder.Decode(&i)
	return i, err
}

func XmlLoads(v interface{}) map[string]any {
	i, err := XmlLoadsOmap(v)
	if err != nil {
		log.Debugf("xml decode error: %v", err)
	}
	return i.ToStringMap()
}

func XmlPrettify(b []byte) string {
	v, _ := XmlLoadsOmap(b)
	return string(XmlDumps(v, WithHTMLEscape(false)))
}
