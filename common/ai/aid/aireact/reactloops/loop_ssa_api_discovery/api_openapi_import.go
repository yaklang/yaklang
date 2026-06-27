package loop_ssa_api_discovery

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/utils"
	yaml "gopkg.in/yaml.v3"
)

const sourceOpenAPISpec = "openapi_spec"

// OpenAPIImportReport OpenAPI/Swagger 导入摘要。
type OpenAPIImportReport struct {
	GeneratedAt      time.Time `json:"generated_at"`
	SpecFilesScanned int       `json:"spec_files_scanned"`
	PathsExtracted   int       `json:"paths_extracted"`
	InsertedRows     int       `json:"inserted_rows"`
	UpdatedRows      int       `json:"updated_rows"`
	TotalEndpoints   int       `json:"total_http_endpoints_after_merge"`
	Warnings         []string  `json:"warnings,omitempty"`
}

var httpVerbs = map[string]struct{}{
	"get": {}, "post": {}, "put": {}, "delete": {}, "patch": {}, "head": {}, "options": {},
}

// RunOpenAPIImportForSession 扫描代码根下 OpenAPI/Swagger JSON/YAML，解析 paths 并合并入 http_endpoints。
func RunOpenAPIImportForSession(rt *Runtime) (*OpenAPIImportReport, error) {
	if rt == nil || rt.Repo == nil || rt.Session == nil {
		return nil, utils.Error("nil runtime")
	}
	sess := rt.Session
	if !sess.CodePathOK || strings.TrimSpace(sess.CodeRootPath) == "" {
		return nil, utils.Errorf("invalid code root")
	}
	root := filepath.Clean(sess.CodeRootPath)
	rep := &OpenAPIImportReport{GeneratedAt: time.Now().UTC()}
	var rows []HarvestedEndpoint
	var warn []string

	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil || info.IsDir() {
			if info != nil && info.IsDir() && skipDirForHarvest(info.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)
		lb := strings.ToLower(filepath.Base(rel))
		if !looksOpenAPIFilename(lb) && !strings.Contains(strings.ToLower(rel), "openapi") && !strings.Contains(strings.ToLower(rel), "swagger") {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if ext != ".json" && ext != ".yaml" && ext != ".yml" {
			return nil
		}
		m, perr := parseSpecFile(path)
		if perr != nil || m == nil {
			return nil
		}
		rep.SpecFilesScanned++
		basePath := ""
		if bp, ok := m["basePath"].(string); ok && strings.TrimSpace(bp) != "" {
			basePath = strings.TrimSpace(bp)
		}
		if basePath == "" {
			basePath = extractOpenAPI3BasePath(m)
		}
		eps, w := extractPathsFromSpecMap(m, rel, basePath)
		if len(w) > 0 {
			warn = append(warn, w...)
		}
		rows = append(rows, eps...)
		return nil
	})

	if len(rows) == 0 {
		rep.Warnings = warn
		b, _ := json.Marshal(rep)
		sess.ApiSpecImportMetaJSON = string(b)
		_ = rt.Repo.UpdateSession(sess)
		rt.Session = sess
		return rep, nil
	}

	rows = dedupeHarvested(rows)
	ins, upd, err := MergeHarvestedHttpEndpoints(rt.Repo, sess.ID, rows)
	if err != nil {
		return nil, err
	}
	rep.InsertedRows = ins
	rep.UpdatedRows = upd
	rep.PathsExtracted = len(rows)
	all, _ := rt.Repo.ListHttpEndpoints(sess.ID)
	rep.TotalEndpoints = len(all)
	rep.Warnings = warn

	b, _ := json.Marshal(rep)
	sess.ApiSpecImportMetaJSON = string(b)
	if err := rt.Repo.UpdateSession(sess); err != nil {
		return rep, err
	}
	rt.Session = sess
	return rep, nil
}

func parseSpecFile(path string) (map[string]interface{}, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(b) > 4*1024*1024 {
		return nil, utils.Errorf("spec too large")
	}
	ext := strings.ToLower(filepath.Ext(path))
	var m map[string]interface{}
	switch ext {
	case ".json":
		if err := json.Unmarshal(b, &m); err != nil {
			return nil, err
		}
	case ".yaml", ".yml":
		if err := yaml.Unmarshal(b, &m); err != nil {
			return nil, err
		}
	default:
		return nil, nil
	}
	return m, nil
}

func extractPathsFromSpecMap(m map[string]interface{}, fileRel, swagger2Base string) ([]HarvestedEndpoint, []string) {
	var out []HarvestedEndpoint
	var warn []string
	pathsObj, ok := m["paths"].(map[string]interface{})
	if !ok {
		return nil, nil
	}
	base := swagger2Base
	if base != "" && !strings.HasPrefix(base, "/") {
		base = "/" + base
	}
	for pRaw, item := range pathsObj {
		p := strings.TrimSpace(pRaw)
		if p == "" {
			continue
		}
		full := joinOpenAPIPath(base, p)
		opMap, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		for k, v := range opMap {
			lk := strings.ToLower(strings.TrimSpace(k))
			if _, isVerb := httpVerbs[lk]; !isVerb {
				continue
			}
			if _, ok := v.(map[string]interface{}); !ok && v != nil {
				continue
			}
			oid := ""
			if vm89, ok := v.(map[string]interface{}); ok {
				if s, ok := vm89["operationId"].(string); ok {
					oid = s
				}
			}
			out = append(out, HarvestedEndpoint{
				Method:        strings.ToUpper(lk),
				PathPattern:   full,
				HandlerClass:  fileRel,
				HandlerMethod: oid,
				Provenance:    sourceOpenAPISpec,
				FileRelPath:   fileRel,
			})
		}
	}
	if len(pathsObj) > 0 && len(out) == 0 {
		warn = append(warn, fmt.Sprintf("spec %s has paths but no HTTP verbs parsed", fileRel))
	}
	return dedupeHarvested(out), warn
}

func joinOpenAPIPath(base, p string) string {
	p = strings.TrimSpace(p)
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	base = strings.TrimSuffix(strings.TrimSpace(base), "/")
	if base == "" {
		return p
	}
	return base + p
}

// extractOpenAPI3BasePath extracts the path portion from OpenAPI 3.0 servers[0].url.
func extractOpenAPI3BasePath(m map[string]interface{}) string {
	servers, ok := m["servers"]
	if !ok {
		return ""
	}
	arr, ok := servers.([]interface{})
	if !ok || len(arr) == 0 {
		return ""
	}
	first, ok := arr[0].(map[string]interface{})
	if !ok {
		return ""
	}
	urlStr, ok := first["url"].(string)
	if !ok || strings.TrimSpace(urlStr) == "" {
		return ""
	}
	urlStr = strings.TrimSpace(urlStr)
	// Extract just the path portion
	for _, prefix := range []string{"https://", "http://"} {
		if strings.HasPrefix(strings.ToLower(urlStr), prefix) {
			rest := urlStr[len(prefix):]
			idx := strings.Index(rest, "/")
			if idx >= 0 {
				return rest[idx:]
			}
			return ""
		}
	}
	if strings.HasPrefix(urlStr, "/") {
		return urlStr
	}
	return ""
}
