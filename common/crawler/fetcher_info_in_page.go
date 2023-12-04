package crawler

import (
	"bytes"
	"fmt"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"golang.org/x/net/html"
	"mime"
	"strconv"
	"strings"
)

type JavaScriptContent struct {
	IsCodeText bool
	Code       string
	UrlPath    string
	Node       *html.Node
}

func (s *JavaScriptContent) String() string {
	if s.IsCodeText {
		if len(s.Code) > 100 {
			return fmt.Sprintf("[code]: %v...", strings.TrimRight(strconv.Quote(s.Code[:100]), `"`))
		}
		return fmt.Sprintf("[code]: %v", strconv.Quote(s.Code))
	}
	return fmt.Sprintf(" [uri]: %v", strconv.Quote(s.UrlPath))
}

func iterHtmlNode(node *html.Node, f func(node *html.Node)) {
	if node == nil {
		return
	}
	f(node)
	iterHtmlNode(node.FirstChild, f)
	iterHtmlNode(node.NextSibling, f)
}

type infoFetcherConfig struct {
	onHtmlTag    []func(tagName string, node *html.Node)
	onHtmlText   []func(node *html.Node)
	onJavaScript []func(content *JavaScriptContent)

	jsContentStack *utils.Stack[*JavaScriptContent]
}

type PageInfoFetchOption func(config *infoFetcherConfig)

func WithFetcher_HtmlTag(f func(string, *html.Node)) PageInfoFetchOption {
	return func(config *infoFetcherConfig) {
		config.onHtmlTag = append(config.onHtmlTag, f)
	}
}

func WithFetcher_HtmlText(f func(*html.Node)) PageInfoFetchOption {
	return func(config *infoFetcherConfig) {
		config.onHtmlText = append(config.onHtmlText, f)
	}
}

func WithFetcher_JavaScript(f func(content *JavaScriptContent)) PageInfoFetchOption {
	return func(config *infoFetcherConfig) {
		config.onJavaScript = append(config.onJavaScript, f)
	}
}

func (c *infoFetcherConfig) callbackJavaScriptUrl(url string, node *html.Node, isDefer bool) {
	content := &JavaScriptContent{
		IsCodeText: false,
		UrlPath:    url,
		Node:       node,
	}
	if isDefer {
		c.jsContentStack.Push(content)
		return
	}
	for _, f := range c.onJavaScript {
		f(content)
	}
}

func (c *infoFetcherConfig) callbackJavaScriptCode(code string, node *html.Node, isDefer bool) {
	content := &JavaScriptContent{
		IsCodeText: true,
		Code:       code,
		Node:       node,
	}

	if isDefer {
		c.jsContentStack.Push(content)
		return
	}
	for _, f := range c.onJavaScript {
		f(content)
	}
}

func (p *infoFetcherConfig) done() {
	if p.jsContentStack != nil {
		for !p.jsContentStack.IsEmpty() {
			content := p.jsContentStack.Pop()
			for _, f := range p.onJavaScript {
				f(content)
			}
		}
	}
}

func PageInformationWalker(mimeType string, page string, opts ...PageInfoFetchOption) error {
	config := &infoFetcherConfig{
		jsContentStack: utils.NewStack[*JavaScriptContent](),
	}
	for _, p := range opts {
		p(config)
	}

	defer func() {
		config.done()
	}()

	t, _, _ := mime.ParseMediaType(mimeType)
	t = strings.ToLower(t)
	if utils.MatchAnyOfSubString(t, "javascript") {
		config.callbackJavaScriptCode(page, nil, false)
		return nil
	}

	node, err := html.Parse(bytes.NewBufferString(page))
	if err != nil {
		return err
	}
	iterHtmlNode(node, func(node *html.Node) {
		switch node.Type {
		case html.ElementNode:
			tagName := strings.ToLower(node.Data)
			for _, f := range config.onHtmlTag {
				f(tagName, node)
			}
			switch tagName {
			case "script":
				var isDefer bool
				var src string
				for _, attr := range node.Attr {
					if strings.ToLower(attr.Key) == "src" && attr.Val != "" && !strings.HasPrefix(strings.TrimSpace(attr.Val), "#") {
						src = attr.Val
					}
					if attr.Key == "defer" {
						isDefer = true
					}
				}

				if src != "" {
					config.callbackJavaScriptUrl(src, node, isDefer)
				}
				if node.FirstChild != nil {
					code := node.FirstChild.Data
					if strings.TrimSpace(code) != "" {
						config.callbackJavaScriptCode(code, node, isDefer)
					}
				}
			}
		case html.TextNode:
			for _, f := range config.onHtmlText {
				f(node)
			}
		case html.DocumentNode:
			log.Debugf("document: %v", node.Data)
		case html.DoctypeNode:
			log.Debugf("doctype: %v", node.Data)
		case html.CommentNode:
			log.Debugf("comment: %v", node.Data)
		case html.ErrorNode:
			log.Debugf("error node: %v", node.Data)
		default:
			log.Debugf("unknown node: %v", node)
		}
	})
	return nil
}
