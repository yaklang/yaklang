// Package simulator
// @Author bcy2007  2023/8/17 16:18
package simulator

import (
	"github.com/PuerkitoBio/goquery"
	"github.com/go-rod/rod"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func ParseProxyStringToUrl(address, username, password string) *url.URL {
	if address == "" {
		return nil
	}
	proxyUrl, err := url.Parse(address)
	if err != nil {
		return nil
	}
	if username != "" || password != "" {
		proxyUser := url.UserPassword(username, password)
		proxyUrl.User = proxyUser
	}
	return proxyUrl
}

func StringArrayContains(array []string, element string) bool {
	for _, s := range array {
		if element == s {
			return true
		}
	}
	return false
}

func ArrayStringContains(array []string, element string) bool {
	for _, s := range array {
		if strings.Contains(element, s) {
			return true
		}
	}
	return false
}

func ArrayInArray(targets, origin []string) bool {
	if len(targets) > len(origin) {
		return false
	}
	for _, target := range targets {
		if !StringArrayContains(origin, target) {
			return false
		}
	}
	return true
}

func GetRepeatStr(origin, source string) string {
	originBytes := []byte(origin)
	sourceBytes := []byte(source)
	var maxTemp []byte
	for num, ob := range originBytes {
		i := 1
		if ob != sourceBytes[0] {
			continue
		}
		temp := []byte{ob}
		if num+i < len(originBytes) {
			for originBytes[num+i] == sourceBytes[i] {
				temp = append(temp, originBytes[num+i])
				i++
				if i >= len(sourceBytes) {
					break
				}
				if num+i >= len(originBytes) {
					break
				}
			}
		}
		if len(temp) >= len(maxTemp) {
			maxTemp = temp
		}
	}
	return string(maxTemp)
}

func connectTest(targetUrl string, proxy *url.URL) error {
	req, err := http.NewRequest("HEAD", targetUrl, nil)
	if err != nil {
		return utils.Error(err)
	}
	opts := []lowhttp.LowhttpOpt{
		lowhttp.WithRequest(req),
		lowhttp.WithTimeout(15 * time.Second),
	}
	if proxy != nil {
		opts = append(opts, lowhttp.WithProxy(proxy.String()))
	}
	_, err = lowhttp.HTTP(opts...)
	if err != nil {
		return utils.Error(err)
	}
	return nil
}

func GetPageSimilarity(pageAHtml, pageBHtml string) float64 {
	docA, err := goquery.NewDocumentFromReader(strings.NewReader(pageAHtml))
	if err != nil {
		return 0.0
	}
	docB, err := goquery.NewDocumentFromReader(strings.NewReader(pageBHtml))
	if err != nil {
		return 0.0
	}
	return getSimRate(docA, docB)
}

func getSimRate(doc1, doc2 *goquery.Document) float64 {
	var domRate, cssRate float64
	domList1, cssList1 := getDomCssList(doc1)
	domList2, cssList2 := getDomCssList(doc2)
	domSimNum := LongestCommonSubsequence(domList1, domList2)
	cssSimNum := LongestCommonSubsequence(cssList1, cssList2)
	domLen := len(domList1) + len(domList2)
	cssLen := len(cssList1) + len(cssList2)
	if domLen == 0 {
		domRate = 0
	} else {
		domRate = float64(2*domSimNum) / float64(domLen)
	}
	if cssLen == 0 {
		cssRate = 0
	} else {
		cssRate = float64(2*cssSimNum) / float64(cssLen)
	}
	return 0.3*domRate + 0.7*cssRate
}

func LongestCommonSubsequence(text1, text2 []string) int {
	m, n := len(text1), len(text2)
	up := make([]int, n+2)
	var a, b, c, tmp, maximum int
	for i := 1; i <= m; i++ {
		for j := 1; j <= n; j++ {
			if text1[i-1] == text2[j-1] {
				tmp = a + 1
			} else {
				tmp = utils.Max(b, c)
			}
			if tmp > maximum {
				maximum = tmp
			}
			c = tmp
			a = b
			up[j] = tmp
			b = up[j+1]
		}
		a = 0
		b = up[1]
		c = 0
	}
	return maximum
}

func getDomCssList(doc *goquery.Document) ([]string, []string) {
	queue := make([]*goquery.Selection, 0)
	domRes, cssRes := make([]string, 0), make([]string, 0)
	queue = append(queue, doc.Selection)
	for len(queue) > 0 {
		curSel := queue[0]
		queue = queue[1:]
		if len(curSel.Nodes) == 0 {
			continue
		}
		for _, c := range curSel.Nodes {
			domRes = append(domRes, c.Data)
			for _, item := range c.Attr {
				key := strings.ToLower(item.Key)
				if key == "class" || key == "style" {
					cssRes = append(cssRes, item.Val)
				}
			}
		}
		queue = append(queue, curSel.Children())
	}
	return domRes[1:], cssRes
}

func ListRemove(targetList []string, obj string) []string {
	for num, item := range targetList {
		if item == obj {
			return append(targetList[:num], targetList[num+1:]...)
		}
	}
	return targetList
}

func ElementsMinus(origins, targets rod.Elements) rod.Elements {
	result := make(rod.Elements, 0)
	for count, origin := range origins {
		flag := false
		for num, target := range targets {
			equal, err := origin.Equal(target)
			if err != nil {
				log.Errorf("check element equal error: %v", err)
				continue
			}
			if equal {
				targets = append(targets[:num], targets[num+1:]...)
				flag = true
				break
			}
		}
		if !flag {
			result = append(result, origin)
		}
		if len(targets) == 0 {
			result = append(result, origins[count+1:]...)
			return result
		}
	}
	return result
}
