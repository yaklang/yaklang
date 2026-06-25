package loop_ssa_api_discovery

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

// HarvestedEndpoint 静态规则产出的候选端点（尚未入库）。
type HarvestedEndpoint struct {
	Method        string
	PathPattern   string
	HandlerClass  string
	HandlerMethod string
	Provenance    string // e.g. static_java_spring
	FileRelPath   string
}

const sourceStaticJavaSpring = "static_java_spring"

var (
	reJavaPackage   = regexp.MustCompile(`(?m)^\s*package\s+([\w.]+)\s*;`)
	reClassDecl     = regexp.MustCompile(`^\s*(?:public\s+)?(?:abstract\s+)?(?:final\s+)?class\s+(\w+)\b`)
	reMappingMethod = regexp.MustCompile(`@(Get|Post|Put|Delete|Patch)Mapping\s*\(\s*(?:(?:value|path)\s*=\s*)?(?:"([^"]*)"|'([^']*)'|)\s*\)`)
	reMappingMethodBare = regexp.MustCompile(`@(Get|Post|Put|Delete|Patch)Mapping\s*$`)
	// RequestMapping("/path") or value|path = "..."
	reReqMapPath = regexp.MustCompile(`@RequestMapping\s*\(\s*(?:(?:value|path)\s*=\s*)?(?:"([^"]*)"|'([^']*)'|)`)
	reReqMethod  = regexp.MustCompile(`method\s*=\s*RequestMethod\.(GET|POST|PUT|DELETE|PATCH|HEAD|OPTIONS)`)
)

func skipDirForHarvest(name string) bool {
	return shouldSkipPathForCodeHarvest(name + "/")
}

const defaultJavaSpringHarvestWorkers = 8

// HarvestJavaSpringMappings 在 codeRoot 下扫描 *.java（多协程分文件解析），用注解启发式抽取 Spring MVC 风格路由。
func HarvestJavaSpringMappings(codeRoot string) ([]HarvestedEndpoint, error) {
	return HarvestJavaSpringMappingsConcurrent(codeRoot, defaultJavaSpringHarvestWorkers)
}

// HarvestJavaSpringMappingsConcurrent workers<=1 时退化为单协程遍历。
func HarvestJavaSpringMappingsConcurrent(codeRoot string, workers int) ([]HarvestedEndpoint, error) {
	if strings.TrimSpace(codeRoot) == "" {
		return nil, utils.Error("empty code root")
	}
	if workers <= 1 {
		return harvestJavaSpringSequential(codeRoot)
	}
	var javaFiles []string
	err := filepath.Walk(codeRoot, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if info.IsDir() {
			if skipDirForHarvest(info.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(strings.ToLower(path), ".java") {
			javaFiles = append(javaFiles, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if len(javaFiles) == 0 {
		return nil, nil
	}
	jobs := make(chan string, len(javaFiles))
	var mu sync.Mutex
	var out []HarvestedEndpoint
	var wg sync.WaitGroup
	n := workers
	if n > len(javaFiles) {
		n = len(javaFiles)
	}
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for abs := range jobs {
				data, rerr := os.ReadFile(abs)
				if rerr != nil {
					continue
				}
				rel, _ := filepath.Rel(codeRoot, abs)
				rel = filepath.ToSlash(rel)
				pkg := ""
				if m := reJavaPackage.FindSubmatch(data); len(m) > 1 {
					pkg = string(m[1])
				}
				eps := harvestSpringFromJavaFile(data, pkg, rel)
				if len(eps) == 0 {
					continue
				}
				mu.Lock()
				out = append(out, eps...)
				mu.Unlock()
			}
		}()
	}
	for _, p := range javaFiles {
		jobs <- p
	}
	close(jobs)
	wg.Wait()
	return dedupeHarvested(out), nil
}

func harvestJavaSpringSequential(codeRoot string) ([]HarvestedEndpoint, error) {
	var out []HarvestedEndpoint
	err := filepath.Walk(codeRoot, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if info.IsDir() {
			if skipDirForHarvest(info.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(strings.ToLower(path), ".java") {
			return nil
		}
		rel, _ := filepath.Rel(codeRoot, path)
		rel = filepath.ToSlash(rel)
		data, rerr := os.ReadFile(path)
		if rerr != nil {
			return nil
		}
		pkg := ""
		if m := reJavaPackage.FindSubmatch(data); len(m) > 1 {
			pkg = string(m[1])
		}
		out = append(out, harvestSpringFromJavaFile(data, pkg, rel)...)
		return nil
	})
	return dedupeHarvested(out), err
}

func harvestSpringFromJavaFile(content []byte, pkg, fileRel string) []HarvestedEndpoint {
	var res []HarvestedEndpoint
	sc := bufio.NewScanner(bytes.NewReader(content))
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var classBase string
	var className string
	var pendingAnnos []string

	flushClassAnnotations := func() {
		classBase = ""
		for _, a := range pendingAnnos {
			classBase = springMergePath(classBase, parseRequestMappingClassPath(a))
		}
		pendingAnnos = pendingAnnos[:0]
	}

	for sc.Scan() {
		line := sc.Text()
		t := strings.TrimSpace(line)
		if t == "" || strings.HasPrefix(t, "//") {
			continue
		}
		if strings.HasPrefix(t, "@") {
			pendingAnnos = append(pendingAnnos, t)
			continue
		}
		if reClassDecl.MatchString(t) {
			flushClassAnnotations()
			if sm := reClassDecl.FindStringSubmatch(t); len(sm) > 1 {
				className = sm[1]
			}
			continue
		}
		// method-like line: possibly preceded by pendingAnnos (mapping)
		if isProbablyMethodDecl(t) {
			annos := pendingAnnos
			pendingAnnos = pendingAnnos[:0]
			javaMethod := extractJavaMethodName(t)
			fq := fqClass(pkg, className)
			for _, ep := range endpointsFromAnnotations(annos, classBase, fq, javaMethod, fileRel) {
				res = append(res, ep)
			}
			continue
		}
		// reset pending if we hit something that's not annotation and not method — likely field/other member
		if !strings.HasPrefix(t, "@") && pendingAnnos != nil {
			// keep class-level stack: if these were not consumed as class-level (before class), drop stale annos
			pendingAnnos = pendingAnnos[:0]
		}
	}
	return res
}

func isProbablyMethodDecl(t string) bool {
	if !strings.Contains(t, "(") {
		return false
	}
	// crude: must have access + type-like + name (
	if strings.HasPrefix(t, "public ") || strings.HasPrefix(t, "protected ") || strings.HasPrefix(t, "private ") {
		return true
	}
	return false
}

func extractJavaMethodName(t string) string {
	// ... Type name(
	idx := strings.Index(t, "(")
	if idx <= 0 {
		return ""
	}
	pre := t[:idx]
	pre = strings.TrimSpace(pre)
	parts := strings.Fields(pre)
	if len(parts) == 0 {
		return ""
	}
	return parts[len(parts)-1]
}

func fqClass(pkg, class string) string {
	if pkg == "" {
		return class
	}
	return pkg + "." + class
}

func parseRequestMappingClassPath(annoLine string) string {
	if !strings.Contains(annoLine, "@RequestMapping") {
		return ""
	}
	paths := extractQuotedPaths(annoLine)
	if len(paths) > 0 {
		return paths[0]
	}
	return ""
}

func endpointsFromAnnotations(annos []string, classBase, fqClass, javaMethod, fileRel string) []HarvestedEndpoint {
	var out []HarvestedEndpoint
	var methodLevelBase string
	for _, a := range annos {
		methodLevelBase = springMergePath(methodLevelBase, parseRequestMappingClassPath(a))
	}
	base := springMergePath(classBase, methodLevelBase)

	for _, a := range annos {
		if sm := reMappingMethod.FindStringSubmatch(a); len(sm) >= 2 {
			m := strings.ToUpper(sm[1])
			p := sm[2]
			if p == "" && len(sm) > 3 {
				p = sm[3]
			}
			full := springJoinPath(base, p)
			out = append(out, HarvestedEndpoint{
				Method:        m,
				PathPattern:   full,
				HandlerClass:  fqClass,
				HandlerMethod: javaMethod,
				Provenance:    sourceStaticJavaSpring,
				FileRelPath:   fileRel,
			})
			continue
		}
		if sm := reMappingMethodBare.FindStringSubmatch(a); len(sm) >= 2 {
			m := strings.ToUpper(sm[1])
			full := springJoinPath(base, "")
			out = append(out, HarvestedEndpoint{
				Method:        m,
				PathPattern:   full,
				HandlerClass:  fqClass,
				HandlerMethod: javaMethod,
				Provenance:    sourceStaticJavaSpring,
				FileRelPath:   fileRel,
			})
			continue
		}
		if strings.Contains(a, "@RequestMapping") {
			paths := extractRequestMappingPaths(a)
			methods := extractRequestMethods(a)
			if len(paths) == 0 {
				paths = []string{""}
			}
			if len(methods) == 0 {
				// 未声明 RequestMethod 时保守记为 GET，避免为每条映射展开 7 种动词导致噪声
				methods = []string{"GET"}
			}
			for _, p := range paths {
				full := springJoinPath(base, p)
				for _, met := range methods {
					out = append(out, HarvestedEndpoint{
						Method:        met,
						PathPattern:   full,
						HandlerClass:  fqClass,
						HandlerMethod: javaMethod,
						Provenance:    sourceStaticJavaSpring,
						FileRelPath:   fileRel,
					})
				}
			}
		}
	}
	return dedupeHarvested(out)
}

func extractRequestMappingPaths(s string) []string {
	// { "/a", "/b" }
	if i := strings.Index(s, "{"); i >= 0 && strings.Contains(s[i:], ",") {
		inner := s[i:]
		return extractQuotedPaths(inner)
	}
	if m := reReqMapPath.FindStringSubmatch(s); len(m) > 0 {
		p := m[1]
		if p == "" && len(m) > 2 {
			p = m[2]
		}
		if p != "" {
			return []string{p}
		}
	}
	return nil
}

func extractRequestMethods(s string) []string {
	ms := reReqMethod.FindAllStringSubmatch(s, -1)
	if len(ms) == 0 {
		return nil
	}
	seen := make(map[string]struct{})
	var out []string
	for _, m := range ms {
		if len(m) < 2 {
			continue
		}
		u := strings.ToUpper(m[1])
		if _, ok := seen[u]; ok {
			continue
		}
		seen[u] = struct{}{}
		out = append(out, u)
	}
	return out
}

func extractQuotedPaths(s string) []string {
	var out []string
	var b strings.Builder
	in := false
	var quote rune
	for _, r := range s {
		if !in {
			if r == '"' || r == '\'' {
				in = true
				quote = r
				b.Reset()
			}
			continue
		}
		if r == quote {
			in = false
			out = append(out, b.String())
			continue
		}
		b.WriteRune(r)
	}
	return out
}

func springJoinPath(base, tail string) string {
	base = normURLPath(base)
	tail = normURLPath(tail)
	if base == "" {
		if tail == "" {
			return "/"
		}
		if !strings.HasPrefix(tail, "/") {
			return collapseDuplicatePathSegments("/" + tail)
		}
		return collapseDuplicatePathSegments(tail)
	}
	if tail == "" || tail == "/" {
		return collapseDuplicatePathSegments(base)
	}
	if strings.HasPrefix(tail, "/") {
		return collapseDuplicatePathSegments(base + tail)
	}
	return collapseDuplicatePathSegments(base + "/" + tail)
}

func collapseDuplicatePathSegments(p string) string {
	p = normURLPath(p)
	if p == "/" {
		return p
	}
	parts := strings.Split(strings.Trim(p, "/"), "/")
	if len(parts) == 0 {
		return "/"
	}
	out := []string{parts[0]}
	for i := 1; i < len(parts); i++ {
		if parts[i] == out[len(out)-1] {
			continue
		}
		out = append(out, parts[i])
	}
	return "/" + strings.Join(out, "/")
}

func springMergePath(a, b string) string {
	if b == "" {
		return a
	}
	return springJoinPath(a, b)
}

func normURLPath(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return ""
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	for strings.Contains(p, "//") {
		p = strings.ReplaceAll(p, "//", "/")
	}
	if p != "/" {
		p = strings.TrimSuffix(p, "/")
	}
	if p == "" {
		return "/"
	}
	return p
}

func dedupeHarvested(in []HarvestedEndpoint) []HarvestedEndpoint {
	key := func(h HarvestedEndpoint) string {
		return strings.ToUpper(h.Method) + "\x00" + normURLPath(h.PathPattern) + "\x00" + h.HandlerClass + "\x00" + h.HandlerMethod
	}
	seen := make(map[string]struct{})
	var out []HarvestedEndpoint
	for _, h := range in {
		k := key(h)
		if _, ok := seen[k]; ok {
			continue
		}
		seen[k] = struct{}{}
		out = append(out, h)
	}
	return out
}

func routeKey(method, path string) string {
	return strings.ToUpper(strings.TrimSpace(method)) + "\x00" + normURLPath(path)
}

// MergeHarvestedHttpEndpoints 将静态产出合并进会话：按 (method, path_pattern) 去重，不覆盖 AI 已有路径上更丰富字段时可选择性补 handler。
// All new inserts go through NormalizeAndValidateEndpoint; invalid candidates are logged and skipped.
func MergeHarvestedHttpEndpoints(repo *store.Repository, sessionID uint, rows []HarvestedEndpoint) (inserted, updated int, err error) {
	if repo == nil {
		return 0, 0, utils.Error("nil repo")
	}
	existing, err := repo.ListHttpEndpoints(sessionID)
	if err != nil {
		return 0, 0, err
	}
	byKey := make(map[string]*store.HttpEndpoint)
	for i := range existing {
		e := &existing[i]
		byKey[routeKey(e.Method, e.PathPattern)] = e
	}
	for _, h := range rows {
		k := routeKey(h.Method, h.PathPattern)
		if cur, ok := byKey[k]; ok {
			if IsAIPrimaryEndpointSource(cur.Source) {
				continue
			}
			need := false
			if cur.HandlerClass == "" && h.HandlerClass != "" {
				cur.HandlerClass = h.HandlerClass
				need = true
			}
			if cur.HandlerMethod == "" && h.HandlerMethod != "" {
				cur.HandlerMethod = h.HandlerMethod
				need = true
			}
			if need {
				if err := repo.UpdateHttpEndpoint(cur); err != nil {
					log.Warnf("ssa_api_discovery: update endpoint: %v", err)
				} else {
					updated++
				}
			}
			continue
		}
		row := &store.HttpEndpoint{
			SessionID:     sessionID,
			Method:        strings.ToUpper(strings.TrimSpace(h.Method)),
			PathPattern:   normURLPath(h.PathPattern),
			HandlerClass:  h.HandlerClass,
			HandlerMethod: h.HandlerMethod,
			AuthzHint:     "",
			Source:        h.Provenance,
			Status:        store.EndpointStatusPendingValidation,
		}
		if row.PathPattern == "" {
			row.PathPattern = "/"
		}
		if reason := NormalizeAndValidateEndpoint(row); reason != "" {
			log.Warnf("ssa_api_discovery: harvest rejected %s %s: %s", row.Method, row.PathPattern, reason)
			continue
		}
		if err := repo.CreateHttpEndpoint(row); err != nil {
			return inserted, updated, fmt.Errorf("create harvested endpoint %s %s: %w", row.Method, row.PathPattern, err)
		}
		byKey[k] = row
		inserted++
	}
	return inserted, updated, nil
}
