package extend

import (
	"github.com/PuerkitoBio/goquery"
	"strings"
	"github.com/yaklang/yaklang/common/utils"
)

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
