package utils

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/utils/htmlquery"
	"golang.org/x/net/html"
	"hash"
	"strings"
)

import (
	twmbMMH3 "github.com/twmb/murmur3"
)

func Mmh3Hash32(raw []byte) string {
	var h32 hash.Hash32 = twmbMMH3.New32()
	_, err := h32.Write([]byte(raw))
	if err == nil {
		return fmt.Sprintf("%d", int32(h32.Sum32()))
	} else {
		//log.Println("favicon Mmh3Hash32 error:", err)
		return "0"
	}
}

func StandBase64(braw []byte) []byte {
	bckd := base64.StdEncoding.EncodeToString(braw)
	var buffer bytes.Buffer
	for i := 0; i < len(bckd); i++ {
		ch := bckd[i]
		buffer.WriteByte(ch)
		if (i+1)%76 == 0 {
			buffer.WriteByte('\n')
		}
	}
	buffer.WriteByte('\n')
	return buffer.Bytes()

}

// ExtractFaviconURL 从 HTML 页面内容中解析出 favicon 图标的 URL，并相对站点地址补全为绝对地址
// 参数:
//   - siteURL: 页面所在的站点地址，用于补全相对路径
//   - content: HTML 页面内容
//
// 返回值:
//   - favicon 图标的绝对 URL
//   - 错误信息，解析失败或未找到图标时返回非空
//
// Example:
// ```
// html = "<html><head><link rel=\"icon\" href=\"/favicon.ico\"></head></html>"
// iconURL = http.ExtractFaviconURL("https://example.com", html)~
// println(iconURL)   // OUT: https://example.com/favicon.ico
// ```
func ExtractFaviconURL(siteURL string, content []byte) (string, error) {
	node, err := htmlquery.Parse(bytes.NewReader(content))
	if err != nil {
		return "", err
	}
	links := htmlquery.Find(node, `//link`)
	var icon = lo.FilterMap(links, func(item *html.Node, index int) (string, bool) {
		var rel = htmlquery.SelectAttr(item, "rel")
		if strings.Contains(rel, "icon") {
			var href = htmlquery.SelectAttr(item, "href")
			if href != "" {
				return href, true
			}
		}
		return "", false
	})

	if len(icon) > 0 {
		return UrlJoin(siteURL, "/"+strings.TrimLeft(icon[0], "/"))
	}

	return "", Errorf("cannot fetch favicon")
}
