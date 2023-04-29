package htmlquery

import (
	"golang.org/x/net/html"
	"yaklang/common/utils"
	"strings"
)

var Exports = map[string]interface{}{
	"LoadHTMLDocument": func(htmlText interface{}) (*html.Node, error) {
		return Parse(strings.NewReader(utils.InterfaceToString(htmlText)))
	},
	"Find":                 Find,
	"FindOne":              FindOne,
	"QueryAll":             QueryAll,
	"Query":                Query,
	"InnerText":            InnerText,
	"SelectAttr":           SelectAttr,
	"ExistedAttr":          ExistsAttr,
	"CreateXPathNavigator": CreateXPathNavigator,

	"OutputHTML": func(doc *html.Node) string {
		return OutputHTML(doc, false)
	},
	"OutputHTMLSelf": func(doc *html.Node) string {
		return OutputHTML(doc, true)
	},
}
