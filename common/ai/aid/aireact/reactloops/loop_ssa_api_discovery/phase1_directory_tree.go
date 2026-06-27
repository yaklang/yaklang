package loop_ssa_api_discovery

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

var (
	backendRootPatterns = []struct {
		Pattern string
		Weight  int
	}{
		{"src/main/java", 10},
		{"src/java", 8},
		{"java/src", 5},
		{"src", 3},
	}
)

type javaRootCandidate struct {
	path  string
	files int
}

// FindJavaRoot programmatically locates the Java source root under projectRoot.
// It prefers src/main/java, picks the one with most files, and excludes test and build directories.
// When multiple candidates exist under the same parent directory, it will trace upward
// to find a common parent that contains multiple backend modules, ensuring we don't
// miss sibling modules in a multi-module project.
func FindJavaRoot(projectRoot string) string {
	if projectRoot == "" {
		return ""
	}
	projectRoot = filepath.Clean(projectRoot)

	var candidates []javaRootCandidate
	var mu sync.Mutex
	var wg sync.WaitGroup

	_ = filepath.Walk(projectRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() {
			return nil
		}

		base := info.Name()
		if base == "test" || base == "tests" || base == "target" || base == "build" ||
			base == "node_modules" || base == ".git" {
			return filepath.SkipDir
		}

		for _, p := range backendRootPatterns {
			if strings.HasSuffix(path, p.Pattern) {
				wg.Add(1)
				go func() {
					defer wg.Done()
					count := countJavaFiles(path)
					mu.Lock()
					candidates = append(candidates, javaRootCandidate{path, count})
					mu.Unlock()
				}()
				return nil
			}
		}
		return nil
	})
	wg.Wait()

	if len(candidates) == 0 {
		return ""
	}

	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].files != candidates[j].files {
			return candidates[i].files > candidates[j].files
		}
		return len(candidates[i].path) < len(candidates[j].path)
	})

	// 优先检查是否有同级目录包含多个 backend module，如果有则追溯到共同父目录
	best := clampJavaRootToProject(candidates[0].path, projectRoot)
	if upwardRoot := traceUpwardToMultiModuleRoot(candidates, projectRoot); upwardRoot != "" {
		best = upwardRoot
	}

	return best
}

// clampJavaRootToProject ensures the resolved Java root never escapes the user-provided code root.
func clampJavaRootToProject(root, projectRoot string) string {
	root = filepath.Clean(root)
	projectRoot = filepath.Clean(projectRoot)
	if root == "" {
		return projectRoot
	}
	if projectRoot == "" {
		return root
	}
	if isDirAncestorOrEqual(projectRoot, root) {
		return root
	}
	log.Warnf("ssa_dir_analysis: clamp Java root %s to project root %s", root, projectRoot)
	return projectRoot
}

// isDirAncestorOrEqual reports whether ancestor is the same as or a parent of descendant.
func isDirAncestorOrEqual(ancestor, descendant string) bool {
	ancestor = filepath.Clean(ancestor)
	descendant = filepath.Clean(descendant)
	if ancestor == descendant {
		return true
	}
	rel, err := filepath.Rel(ancestor, descendant)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// traceUpwardToMultiModuleRoot 检查候选路径中是否有多个 module 共享同一个父目录，
// 如果有则将 root 追溯到该父目录，确保 BFS 能覆盖所有模块。
// 追溯结果不会超过 projectRoot（用户输入的代码根目录）。
func traceUpwardToMultiModuleRoot(candidates []javaRootCandidate, projectRoot string) string {
	if len(candidates) < 2 {
		return ""
	}

	// 按路径深度分组，深度相同意味着它们共享同一个父目录级别
	byDepth := make(map[int][]string)
	for _, c := range candidates {
		depth := strings.Count(c.path, string(filepath.Separator))
		byDepth[depth] = append(byDepth[depth], c.path)
	}

	// 检查每个深度级别，看是否有多个候选路径共享同一个直接父目录
	for _, paths := range byDepth {
		if len(paths) < 2 {
			continue
		}
		// 检查这些路径是否都指向同一个父目录下的不同子目录
		parentSet := make(map[string]int)
		for _, p := range paths {
			parent := filepath.Dir(p)
			parentSet[parent]++
		}
		// 如果有多个不同的父目录，说明这些是兄弟 module，追溯到共同祖先（不超过 projectRoot）
		if len(parentSet) > 1 {
			if commonAncestor := findCommonAncestor(paths, projectRoot); commonAncestor != "" {
				log.Infof("ssa_dir_analysis: traced Java root from %s to multi-module parent %s",
					paths[0], commonAncestor)
				return commonAncestor
			}
		}
	}

	return ""
}

// findCommonAncestor finds the deepest directory that contains all paths, clamped to projectRoot.
func findCommonAncestor(paths []string, projectRoot string) string {
	if len(paths) == 0 {
		return ""
	}
	projectRoot = filepath.Clean(projectRoot)
	common := filepath.Clean(paths[0])
	for _, p := range paths[1:] {
		common = longestCommonAncestorDir(common, filepath.Clean(p))
		if common == "" {
			return projectRoot
		}
	}
	return clampJavaRootToProject(common, projectRoot)
}

// longestCommonAncestorDir returns the deepest directory that is an ancestor of both a and b.
func longestCommonAncestorDir(a, b string) string {
	a = filepath.Clean(a)
	b = filepath.Clean(b)
	for {
		if isDirAncestorOrEqual(a, b) {
			return a
		}
		if isDirAncestorOrEqual(b, a) {
			return b
		}
		pa, pb := filepath.Dir(a), filepath.Dir(b)
		if pa == a && pb == b {
			return ""
		}
		if pa == a {
			a = pb
			continue
		}
		if pb == b {
			b = pa
			continue
		}
		a, b = pa, pb
	}
}

func countJavaFiles(dir string) int {
	count := 0
	_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() && strings.HasSuffix(path, ".java") {
			count++
		}
		return nil
	})
	return count
}

// dirTreeNode is used internally by BuildDirectoryTree.
type dirTreeNode struct {
	node     DirectoryNode
	children []string
}

// BuildDirectoryTree constructs a DirectoryTreeV1 for the given Java root.
// It walks the tree, computes per-node and subtree sizes, and extracts package hints.
func BuildDirectoryTree(javaRoot string) *DirectoryTreeV1 {
	tree := &DirectoryTreeV1{
		SchemaVersion: artifactV2SchemaVersion,
		BackendRoot:   javaRoot,
		Nodes:        []DirectoryNode{},
	}
	if javaRoot == "" {
		return tree
	}

	dirMap := make(map[string]*dirTreeNode)
	relPathToID := make(map[string]string)
	dirMapMu := sync.Mutex{}

	_ = filepath.Walk(javaRoot, func(fullPath string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		if info.IsDir() {
			switch info.Name() {
			case "target", "build", "node_modules", ".git", "test", "tests":
				return filepath.SkipDir
			}
		}

		relPath, _ := filepath.Rel(javaRoot, fullPath)
		if relPath == "." {
			return nil
		}

		if info.IsDir() {
			relPath = filepath.ToSlash(relPath)
			id := uuid.New().String()
			dirMapMu.Lock()
			relPathToID[relPath] = id
			dirMapMu.Unlock()

			parentRel := filepath.Dir(fullPath)
			parentRel, _ = filepath.Rel(javaRoot, parentRel)
			parentRel = filepath.ToSlash(parentRel)
			if parentRel == "." {
				parentRel = ""
			}
			parentID := ""
			dirMapMu.Lock()
			if pid, ok := relPathToID[parentRel]; ok {
				parentID = pid
			}
			dirMapMu.Unlock()

			depth := 0
			for i := 0; i < len(relPath); i++ {
				if relPath[i] == '/' {
					depth++
				}
			}
			dirMapMu.Lock()
			dirMap[relPath] = &dirTreeNode{
				node: DirectoryNode{
					ID:           id,
					ParentID:     parentID,
					RelPath:      relPath,
					Depth:        depth,
					DirectSizeKB: 0,
					TotalSizeKB:  0,
					FileCount:    0,
					FileNames:    []string{},
				},
				children: []string{},
			}
			if parentID != "" {
				if parent, ok := dirMap[parentRel]; ok {
					parent.children = append(parent.children, id)
				}
			}
			dirMapMu.Unlock()
			return nil
		}

		relPath = filepath.ToSlash(relPath)
		parentRel := filepath.Dir(fullPath)
		parentRel, _ = filepath.Rel(javaRoot, parentRel)
		parentRel = filepath.ToSlash(parentRel)
		if parentRel == "." {
			parentRel = ""
		}

		fileSizeKB := info.Size() / 1024

		dirMapMu.Lock()
		if parent, ok := dirMap[parentRel]; ok {
			parent.node.FileCount++
			parent.node.DirectSizeKB += fileSizeKB
			parent.node.FileNames = append(parent.node.FileNames, info.Name())
		}

		// accumulate to ancestors
		for dir := parentRel; dir != ""; {
			if di, ok := dirMap[dir]; ok {
				di.node.TotalSizeKB += fileSizeKB
			}
			dir = filepath.ToSlash(filepath.Dir(dir))
			if dir == "." {
				dir = ""
			}
		}

		// extract package hint from first .java file
		if strings.HasSuffix(fullPath, ".java") {
			if parent, ok := dirMap[parentRel]; ok {
				if parent.node.PackageHint == "" {
					pkg := extractPackageHint(fullPath)
					parent.node.PackageHint = pkg
				}
			}
		}
		dirMapMu.Unlock()
		return nil
	})

	var totalSize int64
	var totalFiles int
	var nodes []DirectoryNode

	dirMapMu.Lock()
	for _, di := range dirMap {
		if di.node.TotalSizeKB == 0 && di.node.DirectSizeKB > 0 {
			di.node.TotalSizeKB = di.node.DirectSizeKB
		}
		if di.node.TotalSizeKB == 0 {
			accumulateChildSizes(di, dirMap)
		}
		totalSize += di.node.TotalSizeKB
		totalFiles += di.node.FileCount
		nodes = append(nodes, di.node)
	}
	dirMapMu.Unlock()

	rootID := uuid.New().String()
	rootNode := DirectoryNode{
		ID:           rootID,
		ParentID:     "",
		RelPath:      "",
		Depth:        0,
		DirectSizeKB: 0,
		TotalSizeKB:  totalSize,
		FileCount:    0,
		FileNames:    []string{},
		PackageHint:  "",
	}
	tree.Nodes = append([]DirectoryNode{rootNode}, nodes...)

	for i := range tree.Nodes {
		if tree.Nodes[i].RelPath != "" {
			parentRel := filepath.Dir(filepath.Join(javaRoot, tree.Nodes[i].RelPath))
			parentRel, _ = filepath.Rel(javaRoot, parentRel)
			parentRel = filepath.ToSlash(parentRel)
			if parentRel == "" || parentRel == "." {
				tree.Nodes[i].ParentID = rootID
			}
		}
	}

	tree.TotalSizeKB = totalSize
	tree.TotalDirs = len(tree.Nodes) - 1
	tree.TotalFiles = totalFiles

	return tree
}

func accumulateChildSizes(di *dirTreeNode, dirMap map[string]*dirTreeNode) {
	var total int64
	for _, childID := range di.children {
		for _, n := range dirMap {
			if n.node.ID == childID {
				if n.node.TotalSizeKB == 0 {
					accumulateChildSizes(n, dirMap)
				}
				total += n.node.TotalSizeKB
				break
			}
		}
	}
	if total > 0 {
		di.node.TotalSizeKB = di.node.DirectSizeKB + total
	}
}

// MarkLeafDirs marks nodes with no subdirectories as bfs:leaf.
func MarkLeafDirs(tree *DirectoryTreeV1) {
	if tree == nil {
		return
	}
	hasChild := make(map[string]bool)
	for _, n := range tree.Nodes {
		if n.ParentID != "" {
			hasChild[n.ParentID] = true
		}
	}
	for i := range tree.Nodes {
		if !hasChild[tree.Nodes[i].ID] {
			if tree.Nodes[i].Analysis == nil {
				tree.Nodes[i].Analysis = &DirAnalysis{}
			}
			tree.Nodes[i].Analysis.BfsControl = BfsControlLeaf
		}
	}
}

// GetBFSLevels returns directories grouped by depth.
// level[0] = root; level[1] = children of root, etc.
func GetBFSLevels(tree *DirectoryTreeV1) [][]string {
	if tree == nil {
		return nil
	}
	byDepth := make(map[int][]string)
	maxDepth := 0
	for _, n := range tree.Nodes {
		byDepth[n.Depth] = append(byDepth[n.Depth], n.ID)
		if n.Depth > maxDepth {
			maxDepth = n.Depth
		}
	}
	levels := make([][]string, maxDepth+1)
	for d := 0; d <= maxDepth; d++ {
		levels[d] = byDepth[d]
	}
	// sort each level by relPath for deterministic order
	for _, level := range levels {
		sort.Slice(level, func(i, j int) bool {
			ni := tree.GetNode(level[i])
			nj := tree.GetNode(level[j])
			if ni == nil || nj == nil {
				return false
			}
			return ni.RelPath < nj.RelPath
		})
	}
	return levels
}

// GetNodeIDsAtDepth returns node IDs at the given depth.
func (t *DirectoryTreeV1) GetNodeIDsAtDepth(depth int) []string {
	var ids []string
	for _, n := range t.Nodes {
		if n.Depth == depth {
			ids = append(ids, n.ID)
		}
	}
	return ids
}

// GetChildren returns child node IDs of the given node.
func (t *DirectoryTreeV1) GetChildren(nodeID string) []string {
	var children []string
	for _, n := range t.Nodes {
		if n.ParentID == nodeID {
			children = append(children, n.ID)
		}
	}
	return children
}

// IsUnderStoppedAncestor reports whether any ancestor has bfs:stop analysis.
func IsUnderStoppedAncestor(tree *DirectoryTreeV1, nodeID string) bool {
	if tree == nil || nodeID == "" {
		return false
	}
	for {
		parent := tree.GetParent(nodeID)
		if parent == nil {
			return false
		}
		if parent.Analysis != nil && parent.Analysis.BfsControl == BfsControlStop {
			return true
		}
		nodeID = parent.ID
	}
}

// ShouldSkipDirAnalysis skips nodes already marked stop or under a stopped ancestor.
func ShouldSkipDirAnalysis(tree *DirectoryTreeV1, nodeID string) bool {
	node := tree.GetNode(nodeID)
	if node == nil {
		return true
	}
	if node.Analysis != nil && node.Analysis.BfsControl == BfsControlStop {
		return true
	}
	return IsUnderStoppedAncestor(tree, nodeID)
}

// GetRootNodeIDs returns node IDs at depth 0 for dynamic BFS seeds.
func GetRootNodeIDs(tree *DirectoryTreeV1) []string {
	if tree == nil {
		return nil
	}
	var ids []string
	for _, n := range tree.Nodes {
		if n.Depth == 0 {
			ids = append(ids, n.ID)
		}
	}
	sort.Strings(ids)
	return ids
}

// CollectNextLevelIDs returns child directory IDs reachable from levelIDs.
// Children are excluded when the parent is bfs:stop or the child is under a stopped ancestor.
func CollectNextLevelIDs(tree *DirectoryTreeV1, levelIDs []string) []string {
	childSet := make(map[string]bool)
	for _, pid := range levelIDs {
		parent := tree.GetNode(pid)
		if parent != nil && parent.Analysis != nil && parent.Analysis.BfsControl == BfsControlStop {
			continue
		}
		for _, cid := range tree.GetChildren(pid) {
			if ShouldSkipDirAnalysis(tree, cid) {
				continue
			}
			childSet[cid] = true
		}
	}

	var result []string
	for id := range childSet {
		result = append(result, id)
	}
	sort.Strings(result)
	return result
}

// BFS control constants
const (
	BfsControlStop    = "bfs:stop"
	BfsControlContinue = "bfs:continue"
	BfsControlLeaf    = "bfs:leaf"
)

// GetNode safely retrieves a node by ID.
func (tree *DirectoryTreeV1) MustGetNode(id string) *DirectoryNode {
	return tree.GetNode(id)
}

// GetParent returns the parent node of the given node, or nil.
func (tree *DirectoryTreeV1) GetParent(nodeID string) *DirectoryNode {
	node := tree.GetNode(nodeID)
	if node == nil || node.ParentID == "" {
		return nil
	}
	return tree.GetNode(node.ParentID)
}

// NewRuntimeError creates a Runtime error.
func NewRuntimeError(format string, args ...any) error {
	return utils.Errorf(format, args...)
}
