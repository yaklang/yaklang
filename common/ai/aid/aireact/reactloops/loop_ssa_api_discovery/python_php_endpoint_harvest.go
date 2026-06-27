package loop_ssa_api_discovery

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/yaklang/yaklang/common/utils"
)

const (
	sourceStaticPythonHTTP = "static_python_http"
	sourceStaticPHPHTTP    = "static_php_http"
)

var (
	rePyFastAPIStarlette = regexp.MustCompile(`@(?:\w+)\.(get|post|put|delete|patch|head|options)\s*\(\s*["']([^'"]+)["']`)
	rePyFlaskRoute       = regexp.MustCompile(`@(?:app|\w+)\.route\s*\(\s*["']([^'"]+)["']`)
	rePyFlaskMethods     = regexp.MustCompile(`(?i)methods\s*=\s*\[([^\]]+)\]`)
	rePHPRoute           = regexp.MustCompile(`Route::(get|post|put|delete|patch|any|match)\s*\(\s*['"]([^'"]+)['"]`)
)

// HarvestPythonHTTPMappings 扫描 *.py：Flask `@app.route`（默认识别为 GET）与 FastAPI/Starlette 风格 `@router.get` 等。
func HarvestPythonHTTPMappings(codeRoot string) ([]HarvestedEndpoint, error) {
	if strings.TrimSpace(codeRoot) == "" {
		return nil, utils.Error("empty code root")
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
		if !strings.HasSuffix(strings.ToLower(path), ".py") {
			return nil
		}
		rel, _ := filepath.Rel(codeRoot, path)
		rel = filepath.ToSlash(rel)
		data, rerr := os.ReadFile(path)
		if rerr != nil {
			return nil
		}
		out = append(out, harvestPythonFromSource(data, rel)...)
		return nil
	})
	return dedupeHarvested(out), err
}

func harvestPythonFromSource(data []byte, fileRel string) []HarvestedEndpoint {
	var res []HarvestedEndpoint
	s := string(data)
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if m := rePyFastAPIStarlette.FindStringSubmatch(line); len(m) >= 3 {
			p := strings.TrimSpace(m[2])
			if p == "" {
				continue
			}
			res = append(res, HarvestedEndpoint{
				Method:        strings.ToUpper(m[1]),
				PathPattern:   p,
				HandlerClass:  "",
				HandlerMethod: "",
				Provenance:    sourceStaticPythonHTTP,
				FileRelPath:   fileRel,
			})
			continue
		}
		if m := rePyFlaskRoute.FindStringSubmatch(line); len(m) >= 2 {
			p := strings.TrimSpace(m[1])
			if p == "" {
				continue
			}
			methods := extractFlaskMethodsFromLine(line)
			if len(methods) == 0 {
				methods = []string{"GET"}
			}
			for _, met := range methods {
				res = append(res, HarvestedEndpoint{
					Method:        met,
					PathPattern:   p,
					HandlerClass:  "",
					HandlerMethod: "",
					Provenance:    sourceStaticPythonHTTP,
					FileRelPath:   fileRel,
				})
			}
		}
	}
	return res
}

func extractFlaskMethodsFromLine(line string) []string {
	if line == "" {
		return nil
	}
	m := rePyFlaskMethods.FindStringSubmatch(line)
	if len(m) < 2 {
		return nil
	}
	inner := m[1]
	var out []string
	for _, part := range strings.FieldsFunc(inner, func(r rune) bool {
		return r == ',' || r == '"' || r == '\'' || r == ' ' || r == '\t'
	}) {
		u := strings.ToUpper(strings.TrimSpace(part))
		switch u {
		case "GET", "POST", "PUT", "DELETE", "PATCH", "HEAD", "OPTIONS":
			out = append(out, u)
		}
	}
	return out
}

// HarvestPHPHTTPMappings 扫描 *.php：Laravel `Route::get('path'` 等常见写法。
func HarvestPHPHTTPMappings(codeRoot string) ([]HarvestedEndpoint, error) {
	if strings.TrimSpace(codeRoot) == "" {
		return nil, utils.Error("empty code root")
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
		if !strings.HasSuffix(strings.ToLower(path), ".php") {
			return nil
		}
		rel, _ := filepath.Rel(codeRoot, path)
		rel = filepath.ToSlash(rel)
		data, rerr := os.ReadFile(path)
		if rerr != nil {
			return nil
		}
		s := string(data)
		for _, m := range rePHPRoute.FindAllStringSubmatch(s, -1) {
			if len(m) < 3 {
				continue
			}
			verb := strings.ToUpper(m[1])
			if verb == "ANY" || verb == "MATCH" {
				verb = "GET"
			}
			p := strings.TrimSpace(m[2])
			if p == "" {
				continue
			}
			out = append(out, HarvestedEndpoint{
				Method:        verb,
				PathPattern:   p,
				HandlerClass:  "",
				HandlerMethod: "",
				Provenance:    sourceStaticPHPHTTP,
				FileRelPath:   rel,
			})
		}
		return nil
	})
	return dedupeHarvested(out), err
}
