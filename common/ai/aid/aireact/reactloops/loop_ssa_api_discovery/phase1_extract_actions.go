package loop_ssa_api_discovery

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/utils"
)

func phase1SearchExtractActionOptions() []reactloops.ReActLoopOption {
	return []reactloops.ReActLoopOption{
		buildDiscoverySearchFiles(),
		buildDiscoveryGrepCode(),
		buildExtractSpringRoutes(),
		buildExtractSpringYaml(),
		buildExtractMavenPom(),
		buildExtractGoMod(),
		buildExtractNpmPackage(),
		buildExtractJavaClassMappings(),
		buildDiscoveryMergeEndpoints(),
		buildDiscoveryBuildApiCatalog(),
	}
}

func buildDiscoverySearchFiles() reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopAction(
		"discovery_search_files",
		"Search repo-relative file paths under code_root. Use glob (e.g. **/pom.xml), suffix (.java), or name_contains. Returns paths only; no content analysis.",
		[]aitool.ToolOption{
			aitool.WithStringParam("glob", aitool.WithParam_Description("glob pattern, e.g. **/application*.yml")),
			aitool.WithStringParam("suffix", aitool.WithParam_Description("file suffix e.g. .java")),
			aitool.WithStringParam("name_contains", aitool.WithParam_Description("substring in basename, e.g. swagger")),
			aitool.WithIntegerParam("max_results", aitool.WithParam_Description("max paths to return, default 50")),
		},
		nil,
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			rt, _, ok := mustRT(loop, op)
			if !ok || rt.Session == nil {
				return
			}
			paths, err := searchFilesUnderCodeRoot(rt.Session.CodeRootPath, fileSearchOpts{
				Glob:         action.GetString("glob"),
				Suffix:       action.GetString("suffix"),
				NameContains: action.GetString("name_contains"),
				MaxResults:   action.GetInt("max_results"),
			})
			if err != nil {
				op.Feedback(err.Error())
				op.Continue()
				return
			}
			out := map[string]any{"count": len(paths), "paths": paths}
			b, _ := json.MarshalIndent(out, "", "  ")
			op.Feedback(string(b))
			op.Continue()
		},
	)
}

func buildDiscoveryGrepCode() reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopAction(
		"discovery_grep_code",
		"Regex search in text files under code_root. Returns file, line, matching text.",
		[]aitool.ToolOption{
			aitool.WithStringParam("pattern", aitool.WithParam_Required(true)),
			aitool.WithStringParam("glob", aitool.WithParam_Description("optional path filter glob")),
			aitool.WithIntegerParam("max_matches", aitool.WithParam_Description("default 100")),
		},
		func(l *reactloops.ReActLoop, action *aicommon.Action) error {
			if strings.TrimSpace(action.GetString("pattern")) == "" {
				return utils.Error("pattern required")
			}
			return nil
		},
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			rt, _, ok := mustRT(loop, op)
			if !ok || rt.Session == nil {
				return
			}
			matches, err := grepFilesUnderCodeRoot(
				rt.Session.CodeRootPath,
				action.GetString("pattern"),
				action.GetString("glob"),
				action.GetInt("max_matches"),
			)
			if err != nil {
				op.Feedback(err.Error())
				op.Continue()
				return
			}
			out := map[string]any{"count": len(matches), "matches": matches}
			b, _ := json.MarshalIndent(out, "", "  ")
			op.Feedback(string(b))
			op.Continue()
		},
	)
}

func buildExtractSpringRoutes() reactloops.ReActLoopOption {
	return buildSingleFileExtractAction(
		"extract_spring_routes",
		"Parse Spring MVC mappings from one Java controller file. auto_upsert writes http_endpoints (source=extract_spring).",
		func(b []byte, rel string) (any, error) {
			return extractSpringRoutesFromBytes(b, rel), nil
		},
		func(rt *Runtime, result any, autoUpsert bool) (int, error) {
			routes, _ := result.([]ExtractedRoute)
			if !autoUpsert {
				return 0, nil
			}
			return upsertExtractedRoutes(rt, routes, SourceExtractSpring)
		},
	)
}

func buildExtractSpringYaml() reactloops.ReActLoopOption {
	return buildSingleFileExtractAction(
		"extract_spring_yaml",
		"Parse application.yml/properties for context-path and server port. auto_upsert writes config_artifacts.",
		func(b []byte, rel string) (any, error) {
			return extractSpringYamlFromBytes(b, rel), nil
		},
		func(rt *Runtime, result any, autoUpsert bool) (int, error) {
			y, ok := result.(ExtractSpringYamlResult)
			if !ok || !autoUpsert {
				return 0, nil
			}
			summary := fmt.Sprintf("context_path=%s port=%s", y.ContextPath, y.ServerPort)
			if err := upsertExtractedConfigArtifact(rt, y.FileRelPath, "yaml", summary); err != nil {
				return 0, err
			}
			return 1, nil
		},
	)
}

func buildExtractMavenPom() reactloops.ReActLoopOption {
	return buildSingleFileExtractAction(
		"extract_maven_pom",
		"Parse pom.xml dependencies. auto_upsert appends dependency_refs.",
		func(b []byte, _ string) (any, error) {
			return extractMavenPomFromBytes(b), nil
		},
		func(rt *Runtime, result any, autoUpsert bool) (int, error) {
			deps, _ := result.([]ExtractedDependency)
			if !autoUpsert {
				return 0, nil
			}
			return upsertExtractedDependencies(rt, deps)
		},
	)
}

func buildExtractGoMod() reactloops.ReActLoopOption {
	return buildSingleFileExtractAction(
		"extract_go_mod",
		"Parse go.mod require directives. auto_upsert appends dependency_refs.",
		func(b []byte, _ string) (any, error) {
			return extractGoModFromBytes(b), nil
		},
		func(rt *Runtime, result any, autoUpsert bool) (int, error) {
			deps, _ := result.([]ExtractedDependency)
			if !autoUpsert {
				return 0, nil
			}
			return upsertExtractedDependencies(rt, deps)
		},
	)
}

func buildExtractNpmPackage() reactloops.ReActLoopOption {
	return buildSingleFileExtractAction(
		"extract_npm_package",
		"Parse package.json dependencies. auto_upsert appends dependency_refs.",
		func(b []byte, _ string) (any, error) {
			return extractNpmPackageFromBytes(b), nil
		},
		func(rt *Runtime, result any, autoUpsert bool) (int, error) {
			deps, _ := result.([]ExtractedDependency)
			if !autoUpsert {
				return 0, nil
			}
			return upsertExtractedDependencies(rt, deps)
		},
	)
}

func buildExtractJavaClassMappings() reactloops.ReActLoopOption {
	return buildSingleFileExtractAction(
		"extract_java_class_mappings",
		"Extract mount prefix / WebMvcConfigurer routing facts from one Java config file.",
		func(b []byte, rel string) (any, error) {
			return extractJavaClassMappingsFromBytes(b, rel), nil
		},
		func(rt *Runtime, result any, autoUpsert bool) (int, error) {
			m, ok := result.(ExtractJavaMappingsResult)
			if !ok || !autoUpsert || len(m.RoutingFacts) == 0 {
				return 0, nil
			}
			summary := fmt.Sprintf("routing_facts=%d", len(m.RoutingFacts))
			if err := upsertExtractedConfigArtifact(rt, m.FileRelPath, "java", summary); err != nil {
				return 0, err
			}
			return len(m.RoutingFacts), nil
		},
	)
}

type fileExtractFn func([]byte, string) (any, error)
type fileExtractUpsertFn func(*Runtime, any, bool) (int, error)

func buildSingleFileExtractAction(name, desc string, extract fileExtractFn, upsert fileExtractUpsertFn) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopAction(
		name,
		desc,
		[]aitool.ToolOption{
			aitool.WithStringParam("file", aitool.WithParam_Required(true), aitool.WithParam_Description("repo-relative path")),
			aitool.WithBoolParam("auto_upsert", aitool.WithParam_Description("write to SQLite (default true)")),
		},
		func(l *reactloops.ReActLoop, action *aicommon.Action) error {
			if strings.TrimSpace(action.GetString("file")) == "" {
				return utils.Error("file required")
			}
			return nil
		},
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			rt, _, ok := mustRT(loop, op)
			if !ok {
				return
			}
			b, rel, err := readRepoRelativeFile(rt, action.GetString("file"))
			if err != nil {
				op.Feedback(fmt.Sprintf("read %s: %v", action.GetString("file"), err))
				op.Continue()
				return
			}
			result, err := extract(b, rel)
			if err != nil {
				op.Feedback(err.Error())
				op.Continue()
				return
			}
			autoUpsert := action.GetBool("auto_upsert", true)
			upserted, uerr := upsert(rt, result, autoUpsert)
			if uerr != nil {
				op.Feedback(uerr.Error())
				op.Continue()
				return
			}
			payload := map[string]any{
				"file": rel, "result": result, "upserted_count": upserted, "auto_upsert": autoUpsert,
			}
			raw, _ := json.MarshalIndent(payload, "", "  ")
			op.Feedback(string(raw))
			op.Continue()
		},
	)
}

func buildDiscoveryMergeEndpoints() reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopAction(
		"discovery_merge_endpoints",
		"Deduplicate http_endpoints in DB by method+path; prefer extract_spring and ai_code_read sources.",
		[]aitool.ToolOption{},
		nil,
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			rt, sess, ok := mustRT(loop, op)
			if !ok {
				return
			}
			merged, err := mergeHttpEndpointsInDB(rt, sess.ID)
			if err != nil {
				op.Feedback(err.Error())
				op.Continue()
				return
			}
			op.Feedback(fmt.Sprintf("merged duplicate endpoints: %d groups", merged))
			op.Continue()
		},
	)
}

func buildDiscoveryBuildApiCatalog() reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopAction(
		"discovery_build_api_catalog",
		"Build api_catalog.json from DB http_endpoints + routing profile (no AI).",
		[]aitool.ToolOption{},
		nil,
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			rt, _, ok := mustRT(loop, op)
			if !ok {
				return
			}
			catalog, err := AssembleApiCatalogFromDB(rt)
			if err != nil {
				op.Feedback(err.Error())
				op.Continue()
				return
			}
			op.Feedback(fmt.Sprintf("api_catalog entries=%d path=%s", len(catalog.Entries), store.ApiCatalogPath(rt.WorkDir)))
			op.Continue()
		},
	)
}

func mergeHttpEndpointsInDB(rt *Runtime, sessionID uint) (int, error) {
	eps, err := rt.Repo.ListHttpEndpoints(sessionID)
	if err != nil {
		return 0, err
	}
	priority := func(src string) int {
		switch strings.ToLower(strings.TrimSpace(src)) {
		case SourceAICodeRead, SourceExtractSpring, "ai":
			return 3
		case SourceStaticHint:
			return 1
		default:
			return 2
		}
	}
	best := map[string]store.HttpEndpoint{}
	dupes := 0
	for _, e := range eps {
		key := strings.ToUpper(e.Method) + " " + e.PathPattern
		ex, ok := best[key]
		if !ok || priority(e.Source) > priority(ex.Source) {
			best[key] = e
			continue
		}
		if e.ID != ex.ID {
			dupes++
			_ = rt.Repo.DeleteHttpEndpoint(sessionID, e.ID)
		}
	}
	return dupes, nil
}
