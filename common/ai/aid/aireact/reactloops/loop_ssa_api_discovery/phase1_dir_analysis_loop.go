package loop_ssa_api_discovery

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

const dirAnalysisConcurrentDefault = 4

func directoryAnalysisConcurrent() int {
	n := dirAnalysisConcurrentDefault
	s := strings.TrimSpace(os.Getenv("YAK_SSA_DIR_ANALYSIS_CONCURRENT"))
	if s == "" {
		return n
	}
	if v, err := strconv.Atoi(s); err == nil && v > 0 {
		return v
	}
	return n
}

func extractPackageHint(filePath string) string {
	f, err := os.Open(filePath)
	if err != nil {
		return ""
	}
	defer f.Close()

	buf := make([]byte, 1024)
	n, _ := f.Read(buf)
	if n == 0 {
		return ""
	}
	content := string(buf[:n])
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "package ") {
			line = strings.TrimPrefix(line, "package ")
			return strings.TrimSuffix(strings.TrimSpace(line), ";")
		}
	}
	return ""
}

// RunDirectoryAnalysis builds the directory tree and runs BFS concurrent analysis.
func RunDirectoryAnalysis(ctx context.Context, r aicommon.AIInvokeRuntime, task aicommon.AIStatefulTask, rt *Runtime) (*DirectoryTreeV1, error) {
	if rt == nil {
		return nil, utils.Error("nil runtime")
	}
	projectRoot := rt.Session.CodeRootPath
	if projectRoot == "" {
		return nil, utils.Error("empty code root path")
	}

	// Step 1: programmatically find Java root
	javaRoot := FindJavaRoot(projectRoot)
	if javaRoot == "" {
		return nil, utils.Errorf("cannot find Java source root under %s", projectRoot)
	}
	log.Infof("ssa_dir_analysis: Java root=%s", javaRoot)

	// Step 2: build directory tree
	treeStart := time.Now()
	rt.execStepStart("phase1.directory_analysis.build_tree", "programmatic")
	tree := BuildDirectoryTree(javaRoot)
	if tree == nil {
		err := utils.Error("failed to build directory tree")
		rt.execStepError("phase1.directory_analysis.build_tree", "programmatic", treeStart, err, nil)
		return nil, err
	}
	rt.execStepEnd("phase1.directory_analysis.build_tree", "programmatic", treeStart, nil)
	log.Infof("ssa_dir_analysis: tree built: %d dirs, %d files, %d KB total",
		tree.TotalDirs, tree.TotalFiles, tree.TotalSizeKB)

	projCtx, _ := loadProjectContextSummary(rt.WorkDir)

	// Step 3: dynamic BFS — only descend when parent is not bfs:stop
	currentLevel := GetRootNodeIDs(tree)
	for levelIdx := 0; len(currentLevel) > 0; levelIdx++ {
		levelStep := fmt.Sprintf("phase1.directory_analysis.bfs_level_%d", levelIdx)
		levelStart := time.Now()
		rt.execStepStart(levelStep, "ai")
		log.Infof("ssa_dir_analysis: analyzing level %d with %d dirs", levelIdx, len(currentLevel))
		if err := concurrentAnalyzeLevel(ctx, r, task, rt, tree, currentLevel, projCtx); err != nil {
			rt.execStepError(levelStep, "ai", levelStart, err, nil)
			log.Warnf("ssa_dir_analysis: level %d error: %v", levelIdx, err)
		} else {
			rt.execStepEnd(levelStep, "ai", levelStart, nil)
		}

		nextIDs := CollectNextLevelIDs(tree, currentLevel)
		if len(nextIDs) == 0 {
			log.Infof("ssa_dir_analysis: BFS finished at level %d", levelIdx)
			break
		}
		currentLevel = nextIDs
	}

	FinalizeDirectoryTreeAnalysis(tree, projCtx, projectRoot)

	// Step 5: persist
	persistStart := time.Now()
	rt.execStepStart("phase1.directory_analysis.persist", "programmatic")
	if err := persistDirectoryTree(rt, tree); err != nil {
		rt.execStepError("phase1.directory_analysis.persist", "programmatic", persistStart, err, nil)
		return nil, utils.Errorf("persist directory tree: %w", err)
	}
	outPath := store.DirectoryAnalysisPath(rt.WorkDir)
	rt.execStepEnd("phase1.directory_analysis.persist", "programmatic", persistStart, []string{outPath})
	log.Infof("ssa_dir_analysis: persisted to %s", outPath)

	return tree, nil
}

func concurrentAnalyzeLevel(ctx context.Context, r aicommon.AIInvokeRuntime, task aicommon.AIStatefulTask, rt *Runtime, tree *DirectoryTreeV1, levelIDs []string, projCtx *ProjectContextSummaryV1) error {
	sem := make(chan struct{}, directoryAnalysisConcurrent())
	var wg sync.WaitGroup
	var mu sync.Mutex
	var firstErr error

	analysisResults := make(map[string]*DirAnalysis)
	var resultMu sync.Mutex
	codeRoot := rt.Session.CodeRootPath

	for _, id := range levelIDs {
		node := tree.GetNode(id)
		if node == nil || ShouldSkipDirAnalysis(tree, id) {
			continue
		}
		if node.Analysis != nil && node.Analysis.BfsControl == BfsControlStop {
			continue // already analyzed as third-party
		}

		wg.Add(1)
		go func(n *DirectoryNode) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			if prog, ok := tryProgrammaticDirAnalysis(n, projCtx, codeRoot); ok {
				prog = sanitizeDirAnalysis(n, projCtx, codeRoot, prog)
				if err := saveDirAnalysisResult(rt, n.ID, prog); err != nil {
					mu.Lock()
					if firstErr == nil {
						firstErr = err
					}
					mu.Unlock()
				}
				resultMu.Lock()
				analysisResults[n.ID] = prog
				resultMu.Unlock()
				return
			}

			analysis, err := runSingleDirAnalysis(ctx, r, task, rt, tree, n, projCtx)
			if analysis != nil {
				analysis = sanitizeDirAnalysis(n, projCtx, codeRoot, analysis)
			}
			resultMu.Lock()
			analysisResults[n.ID] = analysis
			resultMu.Unlock()

			if err != nil {
				mu.Lock()
				if firstErr == nil {
					firstErr = err
				}
				mu.Unlock()
			}
		}(node)
	}
	wg.Wait()

	// Apply results under tree lock
	for id, analysis := range analysisResults {
		for i := range tree.Nodes {
			if tree.Nodes[i].ID == id {
				tree.Nodes[i].Analysis = analysis
				break
			}
		}
	}

	return firstErr
}

func runSingleDirAnalysis(ctx context.Context, r aicommon.AIInvokeRuntime, task aicommon.AIStatefulTask, rt *Runtime, tree *DirectoryTreeV1, node *DirectoryNode, projCtx *ProjectContextSummaryV1) (*DirAnalysis, error) {
	if node == nil {
		return nil, nil
	}

	step := "phase1.directory_analysis.node." + node.ID
	started := time.Now()
	rt.execStepStart(step, "ai")

	if prog, ok := tryProgrammaticDirAnalysis(node, projCtx, rt.Session.CodeRootPath); ok {
		prog = sanitizeDirAnalysis(node, projCtx, rt.Session.CodeRootPath, prog)
		if err := saveDirAnalysisResult(rt, node.ID, prog); err != nil {
			rt.execStepError(step, "ai", started, err, nil)
			return nil, err
		}
		nodeOut := store.DirectoryAnalysisNodePath(rt.WorkDir, node.ID)
		rt.execStepEnd(step, "programmatic", started, []string{nodeOut})
		return prog, nil
	}

	// Build sub-loop
	loop, err := buildDirAnalysisLoop(r, rt, node, tree, projCtx)
	if err != nil {
		rt.execStepError(step, "ai", started, err, nil)
		return nil, err
	}

	subName := "dir_analysis_" + filepath.Base(node.RelPath)
	if subName == "dir_analysis_" {
		subName = "dir_analysis_root"
	}

	if err := runPhase1ReActLoopWithContext(task, subName, loop); err != nil {
		err = utils.Errorf("dir analysis loop %s: %w", subName, err)
		rt.execStepError(step, "ai", started, err, nil)
		return nil, err
	}

	// Read result from file
	nodeOut := store.DirectoryAnalysisNodePath(rt.WorkDir, node.ID)
	analysis, err := loadDirAnalysisResult(rt.WorkDir, node.ID)
	if err != nil {
		// Try to get from loop context
		raw := strings.TrimSpace(loop.Get("dir_analysis_result"))
		if raw != "" {
			var a DirAnalysis
			if parseErr := parseAgentJSONObject(raw, &a); parseErr == nil {
				a2 := sanitizeDirAnalysis(node, projCtx, rt.Session.CodeRootPath, &a)
				rt.execStepEnd(step, "ai", started, []string{nodeOut})
				return a2, nil
			}
		}
		err = utils.Errorf("load dir analysis result for %s: %w", node.ID, err)
		rt.execStepError(step, "ai", started, err, nil)
		return nil, err
	}

	rt.execStepEnd(step, "ai", started, []string{nodeOut})
	return sanitizeDirAnalysis(node, projCtx, rt.Session.CodeRootPath, analysis), nil
}

func buildDirAnalysisLoop(r aicommon.AIInvokeRuntime, rt *Runtime, node *DirectoryNode, tree *DirectoryTreeV1, projCtx *ProjectContextSummaryV1) (*reactloops.ReActLoop, error) {
	persistent := buildDirAnalysisPrompt(node, rt, projCtx)

	opts := []reactloops.ReActLoopOption{
		reactloops.WithMaxIterations(ssaDiscoveryMaxIterations(r)),
		reactloops.WithAllowToolCall(false),
		reactloops.WithAllowRAG(false),
		reactloops.WithAllowAIForge(false),
		reactloops.WithAllowPlanAndExec(false),
		reactloops.WithPersistentInstruction(persistent),
		reactloops.WithInitTask(func(loop *reactloops.ReActLoop, task aicommon.AIStatefulTask, op *reactloops.InitTaskOperator) {
			bindDiscoveryRuntimeInLoop(loop, rt)
			loop.Set("dir_node_id", node.ID)
			loop.Set("dir_rel_path", node.RelPath)
			op.NextAction("read_dir_files")
		}),
		buildDirAnalysisReadFiles(rt, node, projCtx),
		buildDirAnalysisReadJavaFile(rt),
		buildFinalizeDirAnalysis(rt, node, projCtx),
	}

	return reactloops.NewReActLoop("ssa_api_discovery_dir_analysis", r, opts...)
}

func buildDirAnalysisPrompt(node *DirectoryNode, rt *Runtime, projCtx *ProjectContextSummaryV1) string {
	var sb strings.Builder
	if ctxBlock := formatProjectContextForDirPrompt(projCtx); ctxBlock != "" {
		sb.WriteString(ctxBlock)
		sb.WriteString("\n")
	}
	sb.WriteString(`You are analyzing a directory for SSA (Software Security Analysis).

## Your Task
Analyze the directory at ` + node.RelPath + ` and output a structured JSON result.

## Directory Context
- Relative Path: ` + node.RelPath + `
- Total Size: ` + fmt.Sprintf("%d KB", node.TotalSizeKB) + `
- File Count: ` + strconv.Itoa(node.FileCount) + `
- Package Hint: ` + node.PackageHint + `
`)

	if len(node.FileNames) > 0 {
		sb.WriteString("- Files: " + strings.Join(node.FileNames, ", ") + "\n")
	}

	sb.WriteString(`
## Analysis Strategy

### Step 1: Boundary check
Use the **Project context** section as the primary boundary reference. Do not guess paths from generic checklists.

- **Path A (third-party/static)**: Use only when evidence shows non-business content — build tooling, vendored frontend assets, language packs, or clearly external library source. No .java files and only static assets is a strong signal.
- **Path B (business code)**: Default for any directory under **/src/main/java/** unless the Java package is clearly vendored (e.g. org.apache.*, com.google.* embedded libs). **No HTTP entry ≠ third-party** — utilities, config, DAO, and bootstrap code are still business code.
- Do **not** set dependency_info for project first-party packages (see Project context package_roots).

If Path A applies → finalize immediately. Do NOT read non-.java files.

### Path A: Third-Party / Static
` + `{"function_desc":"<brief description>","tech_layers":["tech:third_party"],"biz_domains":[],"db_features":[],"bfs_control":"bfs:stop","is_business":false,"is_http_entry":false,"has_db":false,"dependency_info":{"name":"<library name>","group":"<optional group>","description":"<description>"},"http_entry_files":[]}
` + `
Omit dependency_info.version unless you saw a version in file headers; do not use "unknown".

### Path B: Business Code
If the directory contains business logic, use read_java_file on each .java file (first 30 lines only), then determine:

#### tech_layer (required, multiple allowed)
Choose ALL applicable tags:
| Tag | Meaning | How to detect |
|-----|---------|---------------|
| tech:controller | Receives HTTP requests | Has @RestController/@Controller with @RequestMapping |
| tech:service | Business logic layer | @Service annotation or service interface implementation |
| tech:dao | Data access layer | @Repository/JpaRepository/Mapper/MyBatis XML |
| tech:entity | Data model definition | @Entity/@Table with field definitions |
| tech:config | Configuration/initialization | @Configuration/@Enable* |
| tech:filter | Request/response interceptor | Filter/HandlerInterceptor |
| tech:security | Access control | SecurityConfig/@PreAuthorize |
| tech:generated | Auto-generated code | Path contains "generated"/"target" or file header marker |
| tech:other | None of the above | e.g. utility classes, constants |

#### biz_domain (optional)
| Tag | Meaning | Key indicators |
|-----|---------|---------------|
| biz:cms | Content management | content, category, page, template |
| biz:trade | E-commerce/trade | order, payment, trade |
| biz:oauth | Auth & authorization | oauth, login, auth, token |
| biz:user | User management | user, account |
| biz:file | File management | file, upload, oss |
| biz:sys | System management | sys, config, dict |
| biz:log | Log/audit | log, audit |
| biz:api | API layer | controller.api, rest |
| biz:web | Web layer | controller.web |

#### db_feature (optional)
| Tag | Meaning | How to detect |
|-----|---------|---------------|
| db:jpa | JPA ORM | EntityManager, CrudRepository |
| db:mybatis | MyBatis | SqlSession, @Mapper |
| db:sql | Raw SQL | JdbcTemplate |
| db:cache | Cache operations | RedisTemplate, @Cacheable |
| db:transaction | Transaction mgmt | @Transactional |
| db:none | No DB | No database code detected |

#### bfs_control
- Always "bfs:continue" for business code (keep exploring)

### Output JSON for Path B:
` + `{"function_desc":"<1-2 sentence Chinese description>","tech_layers":["<tech:* tags>"],"biz_domains":["<biz:* tags>"],"db_features":["<db:* tags>"],"bfs_control":"bfs:continue","is_business":true,"is_http_entry":<true/false>,"has_db":<true/false>,"http_entry_files":["<controller file names if is_http_entry=true>"]}

Note: api_feature, auth_feature, route_space will be filled in the auth mechanism detection phase. Output empty or leave blank for those fields.

## Important Rules
1. Use read_java_file (NOT read_file) for .java files only — first 30 lines, no offset/chunk reads
2. Under /src/main/java/ → Path B unless clearly vendored external library source
3. Static-only directories with no .java → Path A only when project context or path evidence supports third-party/static
4. If it's business code → read 2-3 representative .java files and output Path B with bfs:continue
5. Do NOT use require_tool or directly_call_tool; only read_dir_files, read_java_file, finalize_directory_analysis
6. Output ONLY the JSON in finalize_directory_analysis, nothing else
`)

	return sb.String()
}

func buildDirAnalysisReadFiles(rt *Runtime, node *DirectoryNode, projCtx *ProjectContextSummaryV1) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopAction(
		"read_dir_files",
		"List .java files in the target directory. If none exist, the directory is auto-classified as static/third-party.",
		[]aitool.ToolOption{},
		nil,
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			nodeID := strings.TrimSpace(loop.Get("dir_node_id"))
			relPath := strings.TrimSpace(loop.Get("dir_rel_path"))
			if nodeID == "" || relPath == "" {
				op.Feedback("missing dir_node_id or dir_rel_path")
				op.Continue()
				return
			}
			runtime := getRuntime(loop)
			if runtime == nil {
				op.Feedback("runtime not set")
				op.Continue()
				return
			}
			codeRoot := runtime.Session.CodeRootPath
			dirPath := filepath.Join(codeRoot, relPath)
			entries, err := os.ReadDir(dirPath)
			if err != nil {
				op.Feedback("cannot read dir: " + err.Error())
				op.Continue()
				return
			}
			var javaFiles []string
			var fileNames []string
			for _, e := range entries {
				if e.IsDir() {
					continue
				}
				fileNames = append(fileNames, e.Name())
				if strings.HasSuffix(e.Name(), ".java") {
					javaFiles = append(javaFiles, e.Name())
				}
			}
			if len(javaFiles) == 0 {
				probe := node
				if probe == nil {
					probe = &DirectoryNode{RelPath: relPath, FileNames: fileNames}
				} else if len(probe.FileNames) == 0 {
					probe.FileNames = fileNames
				}
				if prog, ok := tryProgrammaticDirAnalysis(probe, projCtx, codeRoot); ok {
					prog = sanitizeDirAnalysis(probe, projCtx, codeRoot, prog)
					if err := autoFinalizeDirAnalysis(loop, op, runtime, nodeID, prog); err != nil {
						op.Feedback("auto-finalize failed: " + err.Error())
						op.Continue()
						return
					}
					return
				}
				op.Feedback("no .java files in " + relPath + "; use project context — if third-party/static, finalize Path A; if module container, Path B with bfs:continue")
				op.Continue()
				return
			}
			var sb strings.Builder
			sb.WriteString("Java files found in " + relPath + ":\n")
			for _, f := range javaFiles {
				sb.WriteString("- " + f + "\n")
			}
			sb.WriteString("\nUse read_java_file on each .java file (first 30 lines only), then finalize_directory_analysis with Path B JSON.")
			op.Feedback(sb.String())
			op.Continue()
		},
	)
}

func buildDirAnalysisReadJavaFile(rt *Runtime) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopAction(
		"read_java_file",
		"Read the first 30 lines of a .java file in the current directory. Only .java filenames allowed; offset/chunk reads are forbidden.",
		[]aitool.ToolOption{
			aitool.WithStringParam("file", aitool.WithParam_Required(true), aitool.WithParam_Description("Java file name or repo-relative path ending in .java")),
		},
		nil,
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			relPath := strings.TrimSpace(loop.Get("dir_rel_path"))
			runtime := getRuntime(loop)
			if runtime == nil || relPath == "" {
				op.Feedback("missing runtime or dir_rel_path")
				op.Continue()
				return
			}
			fileParam := strings.TrimSpace(action.GetString("file"))
			if fileParam == "" {
				op.Feedback("read_java_file: missing required param `file`")
				op.Continue()
				return
			}
			if action.GetInt("offset") > 0 || action.GetInt("chunk_size") > 0 || action.GetInt("chunk-size") > 0 {
				op.Feedback("read_java_file: offset/chunk reads are not allowed; read the first 30 lines only")
				op.Continue()
				return
			}
			baseName := filepath.Base(filepath.FromSlash(fileParam))
			if !strings.HasSuffix(strings.ToLower(baseName), ".java") {
				op.Feedback("read_java_file blocked: only .java files are allowed in directory analysis")
				op.Continue()
				return
			}
			fullPath := filepath.Join(runtime.Session.CodeRootPath, relPath, baseName)
			content, err := readJavaFileHead(fullPath, 30)
			if err != nil {
				op.Feedback("read_java_file failed: " + err.Error())
				op.Continue()
				return
			}
			op.Feedback(content)
			op.Continue()
		},
	)
}

func readJavaFileHead(path string, maxLines int) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	lines := strings.Split(string(b), "\n")
	if len(lines) > maxLines {
		lines = lines[:maxLines]
	}
	return strings.Join(lines, "\n"), nil
}

func saveDirAnalysisResult(rt *Runtime, nodeID string, analysis *DirAnalysis) error {
	if rt == nil || analysis == nil {
		return utils.Error("nil runtime or analysis")
	}
	raw, err := json.Marshal(analysis)
	if err != nil {
		return err
	}
	dir := filepath.Join(rt.WorkDir, store.SubDirName(), "directory_analysis")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(dir, nodeID+".json")
	return os.WriteFile(path, raw, 0o644)
}

func autoFinalizeDirAnalysis(loop *reactloops.ReActLoop, op *reactloops.LoopActionHandlerOperator, rt *Runtime, nodeID string, analysis *DirAnalysis) error {
	if err := saveDirAnalysisResult(rt, nodeID, analysis); err != nil {
		return err
	}
	raw, err := json.Marshal(analysis)
	if err != nil {
		return err
	}
	loop.Set("dir_analysis_result", string(raw))
	op.Feedback("no .java files — auto-classified as third-party/static; analysis saved")
	op.Exit()
	return nil
}

func buildFinalizeDirAnalysis(rt *Runtime, node *DirectoryNode, projCtx *ProjectContextSummaryV1) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopAction(
		"finalize_directory_analysis",
		"Submit directory analysis result as JSON and save it to disk. This ends the analysis.",
		[]aitool.ToolOption{
			aitool.WithStringParam("analysis_json", aitool.WithParam_Required(true)),
		},
		nil,
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			raw := strings.TrimSpace(action.GetString("analysis_json"))
			if raw == "" {
				op.Feedback("analysis_json is required")
				op.Continue()
				return
			}

			var analysis DirAnalysis
			if err := parseAgentJSONObject(raw, &analysis); err != nil {
				op.Feedback("invalid analysis_json: " + err.Error() + " (submit raw JSON without TAG wrappers)")
				op.Continue()
				return
			}

			nodeID := strings.TrimSpace(loop.Get("dir_node_id"))
			if nodeID == "" {
				op.Feedback("missing dir_node_id")
				op.Continue()
				return
			}
			target := node
			if target == nil {
				target = &DirectoryNode{RelPath: strings.TrimSpace(loop.Get("dir_rel_path"))}
			}
			codeRoot := ""
			if rt != nil && rt.Session != nil {
				codeRoot = rt.Session.CodeRootPath
			}
			if isFirstPartyJavaPath(target.RelPath, projCtx) && analysis.BfsControl == BfsControlStop && !isVendoredJavaPath(target.RelPath, projCtx) {
				op.Feedback("first-party Java directory cannot use bfs:stop; use Path B with bfs:continue and is_business=true")
				op.Continue()
				return
			}
			analysisPtr := sanitizeDirAnalysis(target, projCtx, codeRoot, &analysis)

			if err := saveDirAnalysisResult(rt, nodeID, analysisPtr); err != nil {
				op.Feedback("write file: " + err.Error())
				op.Continue()
				return
			}
			path := store.DirectoryAnalysisNodePath(rt.WorkDir, nodeID)

			saved, _ := json.Marshal(analysisPtr)
			loop.Set("dir_analysis_result", string(saved))
			op.Feedback("analysis saved to " + path)
			op.Exit()
		},
	)
}

func loadDirAnalysisResult(workDir string, nodeID string) (*DirAnalysis, error) {
	path := store.DirectoryAnalysisNodePath(workDir, nodeID)
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var analysis DirAnalysis
	if err := json.Unmarshal(b, &analysis); err != nil {
		return nil, err
	}
	return &analysis, nil
}

// runPhase1ReActLoopWithContext runs a sub ReAct loop with the given context.
func runPhase1ReActLoopWithContext(parent aicommon.AIStatefulTask, subName string, loop *reactloops.ReActLoop) error {
	if parent == nil {
		return utils.Error("nil parent task")
	}
	if loop == nil {
		return utils.Error("nil react loop")
	}
	detached, cancel := detachPhase1ReactContext(parent.GetContext())
	defer cancel()

	subID := parent.GetId() + "-" + subName
	sub := aicommon.NewStatefulTaskBase(subID, parent.GetUserInput(), detached, parent.GetEmitter(), true)
	log.Infof("ssa_dir_analysis: running sub-loop %s", subName)
	return loop.ExecuteWithExistedTask(sub)
}
