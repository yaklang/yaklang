package utils

import (
	"bytes"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/utils/htmlquery"
	"golang.org/x/net/html"
	"hash"
	"io/ioutil"
	"net/http"
	"strings"
	"time"
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

func standBase64(braw []byte) []byte {
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

func CalcFaviconHash(urlRaw string) (string, error) {
	timeout := time.Duration(8 * time.Second)
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	client := http.Client{
		Timeout:   timeout,
		Transport: tr,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse /* 不进入重定向 */
		},
	}
	resp, err := client.Get(urlRaw)
	if err != nil {
		//log.Println("favicon client error:", err)
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			//log.Println("favicon file read error: ", err)
			return "", err
		}
		return Mmh3Hash32(standBase64(body)), nil
	} else {
		return "", Errorf("status code: %v", resp.StatusCode)
	}
}

// ExtractFaviconURL will receive a site url and html content return the favicon url
// Example:
//
//	http.ExtractFaviconURL("https://www.baidu.com", []byte(`<link rel="shortcut icon" href="/favicon.ico" type="image/x-icon">`))
//	http.ExtractFaviconURL("https://www.baidu.com", []byte(`<link rel="icon" href="/favicon.ico" type="image/x-icon">`))
//	http.ExtractFaviconURL("https://www.baidu.com", []byte(`<link rel="icon" href="/favicon.png" type="image/png">`))
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
