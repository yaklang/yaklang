package loop_ssa_api_discovery

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
	"github.com/yaklang/yaklang/common/utils"
)

func scanAuthSurfaceGo(rt *Runtime) (string, error) {
	if rt == nil || rt.Session == nil {
		return "", utils.Error("nil runtime")
	}
	root := rt.Session.CodeRootPath
	hints := []map[string]string{}
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil || info.IsDir() {
			return nil
		}
		low := strings.ToLower(path)
		if strings.Contains(low, "node_modules") || strings.Contains(low, ".git") {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(low, ".java") && !strings.HasSuffix(low, ".go") && !strings.HasSuffix(low, ".py") {
			return nil
		}
		b, rerr := os.ReadFile(path)
		if rerr != nil || len(b) > 256000 {
			return nil
		}
		s := string(b)
		for _, kw := range []string{"login", "Login", "authenticate", "JWT", "Bearer", "session", "Filter", "Interceptor", "middleware"} {
			if strings.Contains(s, kw) {
				rel, _ := filepath.Rel(root, path)
				hints = append(hints, map[string]string{"file": rel, "keyword": kw})
				break
			}
		}
		if len(hints) >= 40 {
			return filepath.SkipAll
		}
		return nil
	})
	payload := map[string]any{
		"source": "go_fallback",
		"hints":  hints,
		"count":  len(hints),
	}
	b, _ := json.MarshalIndent(payload, "", "  ")
	if err := writeJSONFile(store.AuthSurfacePath(rt.WorkDir), b); err != nil {
		return "", err
	}
	return store.AuthSurfacePath(rt.WorkDir), nil
}

func scanDependenciesGo(rt *Runtime) (string, error) {
	if rt == nil || rt.Session == nil {
		return "", utils.Error("nil runtime")
	}
	root := rt.Session.CodeRootPath
	payload := buildDependenciesPayloadFromRoot(root)
	payload["source"] = "go_fallback"
	b, _ := json.MarshalIndent(payload, "", "  ")
	if err := writeJSONFile(store.DependenciesInventoryPath(rt.WorkDir), b); err != nil {
		return "", err
	}
	return store.DependenciesInventoryPath(rt.WorkDir), nil
}
