// Package crawlerx
// @Author bcy2007  2023/7/13 11:57
package crawlerx

import (
	"fmt"
	"github.com/yaklang/yaklang/common/log"
	"net/url"
	"regexp"
	"strings"
)

var linkCompilerStr = `((?:[a-zA-Z]{1,10}://|//)[a-zA-Z0-9\-\_]{1,}\.[a-zA-Z]{2,}[^'"\s]{0,})|(\"(?:/|\./|\.\./)[^"'><,;|*()(%%$^/\\\[\]\s][a-zA-Z0-9\-_\.\~\!\*\(\);\:@&\=\+$,\/?#\[\]]{1,}\")|(\'(?:/|\./|\.\./)[^"'><,;|*()(%%$^/\\\[\]\s][a-zA-Z0-9\-_\.\~\!\*\(\);\:@&\=\+$,\/?#\[\]]{1,}\')|href="([a-zA-Z0-9\.\/][^'"\s]*?)"|src="([a-zA-Z0-9\.\/][^'"\s]*?)"`

var tempJsLinkCompilers = []string{
	`\.post\(\s*(\'[^\s]*?\'|\"[^\s]*?\")`,
	`\.get\(\s*(\'[^\s]*?\'|\"[^\s]*?\")`,
	`(?i:url:\s*(\"[^\s]*?\"|\'[^\s]*?\'))`,
	`(?i:url\((\'[^\s]*?\'|\"[^\s]*?\")\))`,
	`(?i:url\([^\'\"\s]*?\))`,
	`(?i:url\s*\=\s*(\'[^\s]*?\'|\"[^\s]*?\"))`,
}

type jsLinkFinder struct {
	Rule   string
	Before int
	After  int
}

var urlChar = `a-zA-Z0-9\.\/\?\_\-\=\&\%\#`

var jsLinkCompilers = []*jsLinkFinder{
	&jsLinkFinder{fmt.Sprintf(`(?i:\.post\(\s*(\'[%s]+?\'|\"[%s]+?\")\,)`, urlChar, urlChar), 8, 2},
	&jsLinkFinder{fmt.Sprintf(`(?i:\.get\(\s*(\'[%s]+?\'|\"[%s]+?\")\,)`, urlChar, urlChar), 7, 2},
	&jsLinkFinder{fmt.Sprintf(`(?i:url:\s*(\"[%s]+?\"|\'[%s]+?\'))`, urlChar, urlChar), 5, 1},
	&jsLinkFinder{fmt.Sprintf(`(?i:url\((\'[%s]+?\'|\"[%s]+?\")\,)`, urlChar, urlChar), 5, 2},
	&jsLinkFinder{fmt.Sprintf(`(?i:url\s*\=\s*(\'[%s]+?\'|\"[%s]+?\"))`, urlChar, urlChar), 5, 1},
}

func analysisHtmlInfo(urlStr, textStr string) []string {
	links := make([]string, 0)
	urlObj, err := url.Parse(urlStr)
	if err != nil {
		return links
	}
	linkCompiler, _ := regexp.Compile(linkCompilerStr)
	originResults := linkCompiler.FindAllString(textStr, -1)
	for _, originResult := range originResults {
		var subString string
		if strings.HasPrefix(originResult, "href") {
			subString = originResult[6 : len(originResult)-1]
		} else if strings.HasPrefix(originResult, "src") {
			subString = originResult[5 : len(originResult)-1]
		} else if strings.HasPrefix(originResult, "\"") || strings.HasPrefix(originResult, "'") {
			subString = originResult[1 : len(originResult)-1]
		} else {
			subString = originResult
		}
		tempObj, err := urlObj.Parse(subString)
		if err != nil {
			log.Errorf("url %s parse %s error: %s", urlObj.String(), subString, err)
			continue
		}
		links = append(links, tempObj.String())
	}
	return links
}

func analysisJsInfo(urlStr, textStr string) []string {
	links := make([]string, 0)
	urlObj, err := url.Parse(urlStr)
	if err != nil {
		return links
	}
	removeSpaceReg, _ := regexp.Compile(`\s+`)
	for _, compiler := range jsLinkCompilers {
		reg, _ := regexp.Compile(compiler.Rule)
		originResults := reg.FindAllString(textStr, -1)
		for _, originResult := range originResults {
			originResult = removeSpaceReg.ReplaceAllString(originResult, ``)
			subString := originResult[compiler.Before : len(originResult)-compiler.After]
			tempObj, err := urlObj.Parse(subString)
			if err != nil {
				log.Errorf("url %s parse %s error: %s", urlObj.String(), subString, err)
				continue
			}
			links = append(links, tempObj.String())
		}
	}
	return links
}
