package loop_ssa_api_discovery

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/yaklang/yaklang/common/utils"
)

const (
	sourceStaticJSHTTP = "static_js_http"
	sourceStaticTSHTTP = "static_ts_http"
)

var (
	reExpressLike = regexp.MustCompile(`(?m)\.(get|post|put|delete|patch|all)\s*\(\s*['"]([^'"]+)['"]\s*`)
	reNestDecor   = regexp.MustCompile(`@(?i)(Get|Post|Put|Delete|Patch)\s*\(\s*['"]([^'"]*)['"]\s*\)`)
)

// HarvestJavaScriptHTTPMappings Express / connect 风格 + 部分 Nest 装饰器（.js/.mjs/.cjs）。
func HarvestJavaScriptHTTPMappings(codeRoot string) ([]HarvestedEndpoint, error) {
	return harvestExpressLikePaths(codeRoot, []string{".js", ".mjs", ".cjs"}, sourceStaticJSHTTP)
}

// HarvestTypeScriptHTTPMappings 同 HarvestJavaScriptHTTPMappings，扩展名 .ts/.tsx/.mts/.cts。
func HarvestTypeScriptHTTPMappings(codeRoot string) ([]HarvestedEndpoint, error) {
	return harvestExpressLikePaths(codeRoot, []string{".ts", ".tsx", ".mts", ".cts"}, sourceStaticTSHTTP)
}

func harvestExpressLikePaths(codeRoot string, exts []string, provenance string) ([]HarvestedEndpoint, error) {
	if strings.TrimSpace(codeRoot) == "" {
		return nil, utils.Error("empty code root")
	}
	extSet := make(map[string]struct{})
	for _, e := range exts {
		extSet[strings.ToLower(e)] = struct{}{}
	}
	var out []HarvestedEndpoint
	err := filepath.Walk(codeRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			if skipDirForHarvest(info.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		lp := strings.ToLower(path)
		if strings.Contains(lp, ".min.") {
			return nil
		}
		ext := filepath.Ext(lp)
		if _, ok := extSet[ext]; !ok {
			return nil
		}
		rel, _ := filepath.Rel(codeRoot, path)
		rel = filepath.ToSlash(rel)
		data, rerr := os.ReadFile(path)
		if rerr != nil {
			return nil
		}
		out = append(out, harvestJS_TSFromSource(data, rel, provenance)...)
		return nil
	})
	return dedupeHarvested(out), err
}

func nestMethodToHTTP(m string) string {
	switch strings.ToUpper(m) {
	case "GET":
		return "GET"
	case "POST":
		return "POST"
	case "PUT":
		return "PUT"
	case "DELETE":
		return "DELETE"
	case "PATCH":
		return "PATCH"
	default:
		return "GET"
	}
}

func harvestJS_TSFromSource(data []byte, fileRel, provenance string) []HarvestedEndpoint {
	var res []HarvestedEndpoint
	s := string(data)
	for _, m := range reExpressLike.FindAllStringSubmatch(s, -1) {
		if len(m) < 3 {
			continue
		}
		path := strings.TrimSpace(m[2])
		if path == "" || strings.HasPrefix(path, ".") {
			continue
		}
		verb := strings.ToUpper(m[1])
		if verb == "ALL" {
			verb = "GET"
		}
		res = append(res, HarvestedEndpoint{
			Method:        verb,
			PathPattern:   path,
			HandlerClass:  "",
			HandlerMethod: "",
			Provenance:    provenance,
			FileRelPath:   fileRel,
		})
	}
	for _, m := range reNestDecor.FindAllStringSubmatch(s, -1) {
		if len(m) < 3 {
			continue
		}
		p := strings.TrimSpace(m[2])
		if p == "" {
			p = "/"
		}
		res = append(res, HarvestedEndpoint{
			Method:        nestMethodToHTTP(m[1]),
			PathPattern:   p,
			HandlerClass:  "",
			HandlerMethod: "",
			Provenance:    provenance,
			FileRelPath:   fileRel,
		})
	}
	return res
}
