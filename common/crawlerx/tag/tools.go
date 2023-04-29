package tag

import (
	"golang.org/x/net/html"
)

func visit(n *html.Node, data, key, value string) bool {
	if n.Type == html.ElementNode && n.Data == data {
		for _, a := range n.Attr {
			if a.Key == key && a.Val == value {
				return true
			}
		}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if visit(c, data, key, value) {
			return true
		}
	}
	return false
}
