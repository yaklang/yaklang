// run_dir_analysis_diagnostic 仅执行 D step（ReAct BFS 目录分析）的树构建与 Work Unit 估算，
// 不触发 ReAct AI 调用，用于评估 D step 在目标项目上的分析效果。
//
// 用法（在 yaklang 仓库根目录）：
//
//	go run ./common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/cmd/run_dir_analysis_diagnostic \
//		-workdir /home/murkfox/yakit-projects/aispace/3434_publiccms_api_scan_20260612_1145e
//
// 输出：
//   - ssa_discovery/directory_tree_preview.json  （树结构预览，含 BFS 层级分组）
//   - ssa_discovery/work_units_preview.json     （Work Unit 估算预览）
//   - stdout: 树统计 + Work Unit 摘要 + 前 20 个 work unit 详情
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	ssa "github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
)

func main() {
	workDir := flag.String("workdir", "", "任务目录（必需，含 ssa_discovery/session.sqlite3）")
	flag.Parse()

	if strings.TrimSpace(*workDir) == "" {
		fmt.Fprintln(os.Stderr, "-workdir is required")
		flag.Usage()
		os.Exit(1)
	}

	// 1. 打开 DB，恢复 session
	db, err := store.OpenSessionDB(*workDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "OpenSessionDB: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = db.Close() }()

	repo := store.NewRepository(db)
	sess, err := repo.GetLatestSession()
	if err != nil {
		fmt.Fprintf(os.Stderr, "GetLatestSession: %v\n", err)
		os.Exit(1)
	}
	if sess == nil {
		fmt.Fprintln(os.Stderr, "no session found")
		os.Exit(1)
	}

	codeRoot := sess.CodeRootPath
	if codeRoot == "" {
		fmt.Fprintln(os.Stderr, "session has no CodeRootPath")
		os.Exit(1)
	}

	fmt.Printf("Session: %s | CodeRoot: %s | Language: %s | Phase: %s\n",
		sess.UUID, codeRoot, sess.Language, sess.Phase)

	// 2. 执行专注模式目录分析（无 AI）
	ctx := context.Background()
	tree, units, err := runDirAnalysisFocus(ctx, *workDir, codeRoot)
	if err != nil {
		fmt.Fprintf(os.Stderr, "run dir analysis focus: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("JavaRoot: %s\n", tree.BackendRoot)
	fmt.Printf("\n=== Directory Tree ===\n")
	fmt.Printf("Total dirs: %d\n", tree.TotalDirs)
	fmt.Printf("Total files: %d\n", tree.TotalFiles)
	fmt.Printf("Total size: %d KB\n", tree.TotalSizeKB)

	levels := ssa.GetBFSLevels(tree)
	fmt.Printf("BFS depth levels: %d\n", len(levels))
	for i, ids := range levels {
		var sizeKB int64
		var files int
		var javaFiles int
		var nonEmpty int
		for _, id := range ids {
			n := tree.GetNode(id)
			if n == nil {
				continue
			}
			sizeKB += n.TotalSizeKB
			files += n.FileCount
			if len(n.FileNames) > 0 {
				nonEmpty++
			}
			for _, fn := range n.FileNames {
				if strings.HasSuffix(fn, ".java") {
					javaFiles++
				}
			}
		}
		fmt.Printf("  Level %d: %d dirs, %d non-empty, %d files, %d java files, %d KB\n",
			i, len(ids), nonEmpty, files, javaFiles, sizeKB)
	}

	fmt.Printf("\n=== Work Unit Estimation ===\n")
	fmt.Printf("Total work units: %d\n", len(units))

	priorityBuckets := map[int]int{1: 0, 2: 0, 3: 0, 4: 0, 5: 0}
	surfaceKinds := map[string]int{}
	var totalKB int64
	for _, u := range units {
		priorityBuckets[u.Priority]++
		surfaceKinds[u.SurfaceKind]++
		totalKB += u.EstimatedKB
	}
	fmt.Printf("Total estimated size: %d KB\n", totalKB)
	fmt.Println("\nBy Priority (1=API+DB, 2=API only, 3=DB only, 4=has API, 5=other):")
	for p := 1; p <= 5; p++ {
		fmt.Printf("  Priority %d: %d work units\n", p, priorityBuckets[p])
	}
	fmt.Println("\nBy SurfaceKind:")
	for k, v := range surfaceKinds {
		fmt.Printf("  %s: %d\n", k, v)
	}

	previews := units
	if len(previews) > 20 {
		previews = previews[:20]
	}
	fmt.Println("\n=== Top 20 Work Units (preview) ===")
	for i, u := range previews {
		fmt.Printf("\n[%d] Priority=%d | Kind=%s | KB=%d\n", i+1, u.Priority, u.SurfaceKind, u.EstimatedKB)
		fmt.Printf("    Label: %s\n", u.Label)
		if len(u.TechLayers) > 0 {
			fmt.Printf("    TechLayers: %v\n", u.TechLayers)
		}
		if len(u.BizDomains) > 0 {
			fmt.Printf("    BizDomains: %v\n", u.BizDomains)
		}
		if len(u.DbFeatures) > 0 {
			fmt.Printf("    DbFeatures: %v\n", u.DbFeatures)
		}
		if len(u.EntryFiles) > 0 {
			fmt.Printf("    EntryFiles: %v\n", u.EntryFiles)
		}
		if len(u.DirIDs) > 0 {
			fmt.Printf("    DirIDs: %d dir(s)\n", len(u.DirIDs))
		}
	}

	// 3. 写预览文件
	subDir := filepath.Join(*workDir, store.SubDirName())
	_ = os.MkdirAll(subDir, 0o755)

	treePath := filepath.Join(subDir, "directory_tree_preview.json")
	treeData, _ := json.MarshalIndent(tree, "", "  ")
	if err := os.WriteFile(treePath, treeData, 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "write tree: %v\n", err)
	} else {
		fmt.Printf("\nTree written to: %s\n", treePath)
	}

	unitsPath := filepath.Join(subDir, "work_units_preview.json")
	unitsData, _ := json.MarshalIndent(units, "", "  ")
	if err := os.WriteFile(unitsPath, unitsData, 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "write units: %v\n", err)
	} else {
		fmt.Printf("Work units written to: %s\n", unitsPath)
	}
}

func runDirAnalysisFocus(ctx context.Context, workDir string, codeRoot string) (*ssa.DirectoryTreeV1, []*ssa.WorkUnit, error) {
	tree, err := ssa.RunDirectoryAnalysisProgrammatic(ctx, workDir, codeRoot)
	if err != nil {
		return nil, nil, fmt.Errorf("run directory analysis programmatic: %w", err)
	}

	units, err := ssa.EstimateWorkUnits(tree)
	if err != nil {
		return nil, nil, fmt.Errorf("estimate work units: %w", err)
	}

	return tree, units, nil
}
