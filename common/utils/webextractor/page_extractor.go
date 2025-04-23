package webextractor

import (
	"errors"
	"fmt"
	"github.com/PuerkitoBio/goquery"
	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"golang.org/x/net/html"
	"regexp"
	"strings"
	"time"
)

func ExtractPageLowhttp(url string) (string, error) {
	lowhttp.HTTP()
	return "", nil
}

func ExtractPageRod(url string) (string, error) {
	extractor := NewRodExtractor()
	if extractor == nil {
		return "", errors.New("extractor is nil")
	}
	return extractor.ExtractFromURL(url)
}

const (
	defaultTimeout = 30 * time.Second
)

type RodExtractor struct {
	browser     *rod.Browser
	pageTimeout time.Duration
	userAgent   string
}

// NewRodExtractor 创建带配置的提取器
func NewRodExtractor() *RodExtractor {
	return &RodExtractor{
		browser:     rod.New().Timeout(defaultTimeout),
		pageTimeout: 15 * time.Second,
		userAgent:   "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36",
	}
}

// ExtractFromURL 使用Rod提取网页内容
func (e *RodExtractor) ExtractFromURL(url string) (string, error) {
	// 启动浏览器实例
	err := e.browser.Connect()
	if err != nil {
		return "", fmt.Errorf("浏览器连接失败: %v", err)
	}
	defer e.browser.MustClose()

	// 创建页面实例
	page := e.browser.MustPage(url)
	defer page.MustClose()

	// 设置浏览器参数
	page = page.Timeout(e.pageTimeout)

	// 智能等待页面加载
	err = rod.Try(func() {
		page.MustWaitLoad().MustWaitIdle().MustSetUserAgent(&proto.NetworkSetUserAgentOverride{UserAgent: e.userAgent})
	})
	if err != nil {
		return "", err
	}

	// 获取渲染后的HTML
	html, err := page.HTML()
	if err != nil {
		return "", fmt.Errorf("获取HTML失败: %v", err)
	}

	// 处理HTML内容
	return processContent(html)
}

// processContent 增强版内容处理
func processContent(html string) (string, error) {
	// 预处理：移除HTML注释
	html = removeComments(html)

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return "", err
	}

	// 多级过滤策略
	doc.Find(`
        script, style, noscript, iframe, 
        svg, link, meta, head,
        [role="alert"], [class*="cookie"], 
        [id^="ads"], [class*="advert"]
    `).Each(func(i int, s *goquery.Selection) {
		s.Remove()
	})

	// 智能正文定位算法
	content := doc.Find("body")
	var buf strings.Builder
	processNode(content.Nodes[0], &buf, 0)

	return strings.TrimSpace(cleanExtraSpaces(buf.String())), nil
}

// 移除HTML注释的正则表达式
func removeComments(html string) string {
	re := regexp.MustCompile(`<!--.*?-->`)
	return re.ReplaceAllString(html, "")
}

func processNode(n *html.Node, buf *strings.Builder, indent int) {
	// 跳过不需要处理的节点类型
	if n.Type == html.CommentNode || n.Type == html.DoctypeNode {
		return
	}
	switch n.Data {
	case "br":
		buf.WriteString("\n")
	case "hr":
		buf.WriteString("\n———\n")
	}
	// 处理文本节点
	if n.Type == html.TextNode {
		text := strings.TrimSpace(n.Data)
		if text != "" {
			buf.WriteString(text)
			buf.WriteString(" ") // 保持单词间距
		}
	}
	// 处理子节点
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		// 块级元素前添加换行和缩进
		if isBlockElement(c) {
			buf.WriteString("\n" + strings.Repeat("  ", indent))
		}
		// 特殊标签处理
		switch c.Data {
		case "h1", "h2", "h3", "h4", "h5", "h6":
			buf.WriteString("\n" + strings.Repeat("#", getHeaderLevel(c.Data)) + " ")
		case "li":
			buf.WriteString("* ")
		case "pre":
			buf.WriteString("\n```\n")
		}
		// 递归处理
		newIndent := indent
		if isContainerElement(c) {
			newIndent = indent + 1
		}
		processNode(c, buf, newIndent)
		// 块级元素后添加换行
		if isBlockElement(c) {
			buf.WriteString("\n")
		}
		switch c.Data {
		case "pre":
			buf.WriteString("\n```\n")
		case "p", "div", "article":
			buf.WriteString("\n")
		}
	}
}

// 辅助函数
func isBlockElement(n *html.Node) bool {
	if n.Type != html.ElementNode {
		return false
	}
	blockTags := []string{"div", "p", "h1", "h2", "h3", "h4", "h5", "h6", "ul", "ol", "li", "blockquote", "pre", "section", "article", "header", "footer", "table", "tr", "td"}
	for _, tag := range blockTags {
		if n.Data == tag {
			return true
		}
	}
	return false
}
func isContainerElement(n *html.Node) bool {
	return n.Type == html.ElementNode && (n.Data == "ul" || n.Data == "ol" || n.Data == "blockquote" || n.Data == "div")
}
func getHeaderLevel(tag string) int {
	if len(tag) == 2 && tag[0] == 'h' {
		return int(tag[1] - '0')
	}
	return 1
}
func cleanExtraSpaces(s string) string {
	// 合并多个换行
	s = strings.ReplaceAll(s, "\n\n\n", "\n\n")
	// 合并多个空格
	return strings.Join(strings.Fields(s), " ")
}
