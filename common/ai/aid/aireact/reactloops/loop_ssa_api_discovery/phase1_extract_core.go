package loop_ssa_api_discovery

import (
	"encoding/json"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
	"github.com/yaklang/yaklang/common/utils"
)

const SourceExtractSpring = "extract_spring"

type ExtractedRoute struct {
	Method        string `json:"method"`
	PathPattern   string `json:"path_pattern"`
	HandlerClass  string `json:"handler_class,omitempty"`
	HandlerMethod string `json:"handler_method,omitempty"`
	FileRelPath   string `json:"file_rel_path"`
	Provenance    string `json:"provenance,omitempty"`
}

type ExtractedDependency struct {
	Name      string `json:"name"`
	Version   string `json:"version"`
	Ecosystem string `json:"ecosystem"`
}

type ExtractSpringYamlResult struct {
	FileRelPath  string        `json:"file_rel_path"`
	ContextPath  string        `json:"context_path,omitempty"`
	ServerPort   string        `json:"server_port,omitempty"`
	Profiles     []string      `json:"profiles,omitempty"`
	RoutingFacts []RoutingFact `json:"routing_facts,omitempty"`
}

type ExtractJavaMappingsResult struct {
	FileRelPath  string        `json:"file_rel_path"`
	RoutingFacts []RoutingFact `json:"routing_facts,omitempty"`
}

var (
	rePomDependency = regexp.MustCompile(`<dependency>\s*[\s\S]*?<groupId>([^<]+)</groupId>[\s\S]*?<artifactId>([^<]+)</artifactId>(?:[\s\S]*?<version>([^<]+)</version>)?`)
	reGoModRequire  = regexp.MustCompile(`(?m)^require\s+(\S+)\s+(\S+)`)
	reNpmDep        = regexp.MustCompile(`"([^"]+)":\s*"([^"]+)"`)
)

func extractSpringRoutesFromBytes(content []byte, fileRel string) []ExtractedRoute {
	pkg := ""
	if m := reJavaPackage.FindStringSubmatch(string(content)); len(m) > 1 {
		pkg = m[1]
	}
	harvested := harvestSpringFromJavaFile(content, pkg, fileRel)
	var out []ExtractedRoute
	for _, h := range harvested {
		out = append(out, ExtractedRoute{
			Method:        h.Method,
			PathPattern:   h.PathPattern,
			HandlerClass:  h.HandlerClass,
			HandlerMethod: h.HandlerMethod,
			FileRelPath:   h.FileRelPath,
			Provenance:    h.Provenance,
		})
	}
	return out
}

func extractSpringYamlFromBytes(content []byte, fileRel string) ExtractSpringYamlResult {
	s := string(content)
	res := ExtractSpringYamlResult{FileRelPath: fileRel}
	for _, m := range reSpringCtxPath.FindAllStringSubmatch(s, -1) {
		if len(m) > 1 {
			res.ContextPath = normURLPath(m[1])
			res.RoutingFacts = append(res.RoutingFacts, RoutingFact{
				Kind: "spring_context_path", MountPrefix: res.ContextPath,
				Ref: fileRel, Hint: "server.servlet.context-path", Confidence: 0.85,
			})
			break
		}
	}
	for _, m := range reServerPort.FindAllStringSubmatch(s, -1) {
		if len(m) > 1 {
			res.ServerPort = strings.TrimSpace(m[1])
			break
		}
	}
	return res
}

func extractJavaClassMappingsFromBytes(content []byte, fileRel string) ExtractJavaMappingsResult {
	return ExtractJavaMappingsResult{
		FileRelPath:  fileRel,
		RoutingFacts: extractMountPrefixesFromJava(string(content), fileRel),
	}
}

func extractMavenPomFromBytes(content []byte) []ExtractedDependency {
	var out []ExtractedDependency
	seen := map[string]struct{}{}
	for _, m := range rePomDependency.FindAllStringSubmatch(string(content), -1) {
		if len(m) < 3 {
			continue
		}
		name := strings.TrimSpace(m[1]) + ":" + strings.TrimSpace(m[2])
		ver := ""
		if len(m) > 3 {
			ver = strings.TrimSpace(m[3])
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, ExtractedDependency{Name: name, Version: ver, Ecosystem: "maven"})
	}
	return out
}

func extractGoModFromBytes(content []byte) []ExtractedDependency {
	var out []ExtractedDependency
	for _, m := range reGoModRequire.FindAllStringSubmatch(string(content), -1) {
		if len(m) < 3 {
			continue
		}
		out = append(out, ExtractedDependency{
			Name: strings.TrimSpace(m[1]), Version: strings.TrimSpace(m[2]), Ecosystem: "go",
		})
	}
	return out
}

func extractNpmPackageFromBytes(content []byte) []ExtractedDependency {
	var raw map[string]any
	if json.Unmarshal(content, &raw) != nil {
		return nil
	}
	deps, _ := raw["dependencies"].(map[string]any)
	var out []ExtractedDependency
	for name, v := range deps {
		out = append(out, ExtractedDependency{
			Name: name, Version: strings.TrimSpace(toString(v)), Ecosystem: "npm",
		})
	}
	dev, _ := raw["devDependencies"].(map[string]any)
	for name, v := range dev {
		out = append(out, ExtractedDependency{
			Name: name, Version: strings.TrimSpace(toString(v)), Ecosystem: "npm",
		})
	}
	return out
}

func toString(v any) string {
	if v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return t
	default:
		b, _ := json.Marshal(t)
		return string(b)
	}
}

func upsertExtractedRoutes(rt *Runtime, routes []ExtractedRoute, source string) (int, error) {
	if rt == nil || rt.Repo == nil || rt.Session == nil {
		return 0, utils.Error("nil runtime")
	}
	if source == "" {
		source = SourceExtractSpring
	}
	inserted := 0
	for _, r := range routes {
		method := strings.ToUpper(strings.TrimSpace(r.Method))
		path := normURLPath(r.PathPattern)
		if method == "" || path == "" {
			continue
		}
		row := &store.HttpEndpoint{
			SessionID:     rt.Session.ID,
			Method:        method,
			PathPattern:   path,
			HandlerClass:  r.HandlerClass,
			HandlerMethod: r.HandlerMethod,
			Source:        source,
			Status:        store.EndpointStatusCandidate,
		}
		res, err := EndpointInsertionGateway(rt, row)
		if err != nil {
			return inserted, err
		}
		if res != nil && res.Reason == "created" {
			inserted++
		}
	}
	return inserted, nil
}

func upsertExtractedDependencies(rt *Runtime, deps []ExtractedDependency) (int, error) {
	if rt == nil || rt.Repo == nil || rt.Session == nil {
		return 0, utils.Error("nil runtime")
	}
	existing, _ := rt.Repo.ListDependencies(rt.Session.ID)
	seen := map[string]struct{}{}
	for _, e := range existing {
		seen[e.Name+"@"+e.Version] = struct{}{}
	}
	added := 0
	for _, d := range deps {
		if d.Name == "" {
			continue
		}
		key := d.Name + "@" + d.Version
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		if err := rt.Repo.CreateDependency(&store.DependencyRef{
			SessionID: rt.Session.ID, Name: d.Name, Version: d.Version, Ecosystem: d.Ecosystem,
		}); err != nil {
			return added, err
		}
		added++
	}
	return added, nil
}

func upsertExtractedConfigArtifact(rt *Runtime, rel, format, summary string) error {
	if rt == nil || rt.Repo == nil || rt.Session == nil {
		return utils.Error("nil runtime")
	}
	if format == "" {
		format = strings.TrimPrefix(filepath.Ext(rel), ".")
	}
	return rt.Repo.CreateConfigArtifact(&store.ConfigArtifact{
		SessionID: rt.Session.ID,
		RelPath:   rel,
		Format:    format,
		Summary:   summary,
	})
}
