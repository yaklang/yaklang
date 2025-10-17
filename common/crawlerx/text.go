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

var linkCompilerStr = `((?:[a-zA-Z]{1,10}://|//)[a-zA-Z0-9\-\_]{1,}\.[a-zA-Z]{2,}[^'"\s]{0,})|(\"(?:/|\./|\.\./)[^"'><,;|*()(%%$^/\\\[\]\s][a-zA-Z0-9\-_\.\~\!\*\(\);\:@&\=\+$,\/?#\[\]]{1,}\")|(\'(?:/|\./|\.\./)[^"'><,;|*()(%%$^/\\\[\]\s][a-zA-Z0-9\-_\.\~\!\*\(\);\:@&\=\+$,\/?#\[\]]{1,}\')|href="([a-zA-Z0-9\.\/][^'"\s]*?)"|src="([a-zA-Z0-9\.\/][^'"\s]*?)"|data-url="([a-zA-Z0-9\.\/][^'"\s]*?)"`

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
	&jsLinkFinder{fmt.Sprintf(`\.post\(\s*(\'[%s]+?\'|\"[%s]+?\")\,`, urlChar, urlChar), 8, 2},
	&jsLinkFinder{fmt.Sprintf(`\.get\(\s*(\'[%s]+?\'|\"[%s]+?\")\,`, urlChar, urlChar), 7, 2},
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
		} else if strings.HasPrefix(originResult, "data-url") {
			subString = originResult[10 : len(originResult)-1]
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
	if strings.HasSuffix(urlStr, ".min.js") {
		return links
	}
	if isPopularJSLibrary(urlStr) {
		return links
	}
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
			var tempObj *url.URL
			tempObj, err = urlObj.Parse(subString)
			if err != nil {
				log.Errorf("url %s parse %s error: %s", urlObj.String(), subString, err)
				continue
			}
			links = append(links, tempObj.String())
		}
	}
	return links
}

var popularJavaScriptLibraryFiles = []string{"react", "vue", "angular", "jquery", "lodash", "bootstrap", "express", "d3", "moment", "axios", "three", "socket.io", "underscore", "ember", "backbone", "redux", "meteor", "next", "nuxt", "gatsby", "svelte", "preact", "material-ui", "ant-design", "bulma", "semantic-ui", "foundation", "tailwind", "styled-components", "apollo", "graphql", "mobx", "knockout", "mithril", "aurelia", "stimulus", "alpine", "inferno", "riot", "cypress", "rxjs", "zone", "hammerjs", "yarn", "npm", "webpack", "babel", "gulp", "grunt", "browserify", "rollup", "eslint", "prettier", "stylelint", "typescript", "coffeescript", "polymer", "lit-element", "lit-html", "stencil", "dojo", "extjs", "raphael", "paper", "fabric", "konva", "anime", "mojs", "velocity", "greensock", "scrollmagic", "popmotion", "lazy", "immutable", "ramda", "bacon", "bluebird", "q", "when", "leaflet", "openlayers", "mapbox-gl", "highcharts", "amcharts", "chart", "echarts", "zrender", "dimple", "c3", "dc", "nvd3", "plottable", "sigma", "vivagraphjs", "jointjs", "cytoscape", "vis", "gojs", "fabric", "paper", "color"}

func isPopularJSLibrary(libraryFileName string) bool {
	for _, lib := range popularJavaScriptLibraryFiles {
		if strings.Contains(lib, strings.ToLower(libraryFileName)) {
			return true
		}
	}
	return false
}
