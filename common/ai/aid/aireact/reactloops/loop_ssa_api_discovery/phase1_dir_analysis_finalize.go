package loop_ssa_api_discovery

import (
	"path/filepath"
	"strings"
)

var vendoredJavaPathPrefixes = []string{
	"/com/google/",
	"/org/apache/",
	"/org/eclipse/",
	"/io/netty/",
	"/javax/",
}

var firstPartyBootstrapPathSuffixes = []string{
	"/src/main/java/boot",
	"/src/main/java/config/initializer",
	"/src/main/java/config/spring",
}

// FinalizeDirectoryTreeAnalysis runs post-BFS cleanup: propagate stop, mark visited leaves.
func FinalizeDirectoryTreeAnalysis(tree *DirectoryTreeV1, projCtx *ProjectContextSummaryV1, codeRoot string) {
	if tree == nil {
		return
	}
	propagateStoppedSubtrees(tree, projCtx, codeRoot)
	markVisitedLeaves(tree)
}

func propagateStoppedSubtrees(tree *DirectoryTreeV1, projCtx *ProjectContextSummaryV1, codeRoot string) {
	if tree == nil {
		return
	}
	childrenByParent := buildChildrenIndex(tree)
	for i := range tree.Nodes {
		n := &tree.Nodes[i]
		if n.Analysis == nil || n.Analysis.BfsControl != BfsControlStop {
			continue
		}
		propagateStopFromNode(n, tree, projCtx, codeRoot, childrenByParent)
	}
}

func propagateStopFromNode(stopNode *DirectoryNode, tree *DirectoryTreeV1, projCtx *ProjectContextSummaryV1, codeRoot string, childrenByParent map[string][]string) {
	if stopNode == nil {
		return
	}
	queue := append([]string(nil), childrenByParent[stopNode.ID]...)
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		child := tree.GetNode(id)
		if child == nil {
			continue
		}
		if child.Analysis != nil && isBFSSanalyzedAnalysis(child.Analysis) {
			queue = append(queue, childrenByParent[id]...)
			continue
		}
		inherited := buildInheritedThirdPartyAnalysis(child, projCtx, codeRoot, stopNode)
		child.Analysis = inherited
		queue = append(queue, childrenByParent[id]...)
	}
}

func buildInheritedThirdPartyAnalysis(node *DirectoryNode, projCtx *ProjectContextSummaryV1, codeRoot string, stopAncestor *DirectoryNode) *DirAnalysis {
	base := buildThirdPartyDirAnalysis(node, projCtx, codeRoot)
	if base == nil {
		base = &DirAnalysis{
			FunctionDesc: "Inherited third-party/static subtree",
			TechLayers:   []string{"tech:third_party"},
			BizDomains:   []string{},
			DbFeatures:   []string{"db:none"},
			BfsControl:   BfsControlStop,
			IsBusiness:   false,
			IsHttpEntry:  false,
			HasDB:        false,
		}
	}
	base.BfsControl = BfsControlStop
	base.IsBusiness = false
	base.IsHttpEntry = false
	base.TechLayers = []string{"tech:third_party"}
	if stopAncestor != nil && stopAncestor.Analysis != nil && stopAncestor.Analysis.DepInfo != nil {
		if base.DepInfo == nil {
			base.DepInfo = &DepInfo{}
		}
		if strings.TrimSpace(base.DepInfo.Name) == "" {
			base.DepInfo.Name = stopAncestor.Analysis.DepInfo.Name
		}
		if strings.TrimSpace(base.DepInfo.Group) == "" {
			base.DepInfo.Group = stopAncestor.Analysis.DepInfo.Group
		}
		if normalizeVersion(base.DepInfo.Version) == "" {
			base.DepInfo.Version = normalizeVersion(stopAncestor.Analysis.DepInfo.Version)
		}
	}
	base.DepInfo = enrichDepInfo(node, codeRoot, base.DepInfo)
	if strings.TrimSpace(base.FunctionDesc) == "" {
		base.FunctionDesc = "Inherited non-business static/third-party content"
	}
	return base
}

func isBFSSanalyzedAnalysis(a *DirAnalysis) bool {
	if a == nil {
		return false
	}
	if a.BfsControl == BfsControlLeaf {
		return true
	}
	if a.BfsControl == BfsControlContinue && strings.TrimSpace(a.FunctionDesc) != "" {
		return true
	}
	if a.BfsControl == BfsControlStop && strings.TrimSpace(a.FunctionDesc) != "" && !isPlaceholderLeafAnalysis(a) {
		return true
	}
	return false
}

func isPlaceholderLeafAnalysis(a *DirAnalysis) bool {
	if a == nil {
		return true
	}
	return a.BfsControl == BfsControlLeaf && strings.TrimSpace(a.FunctionDesc) == "" &&
		len(a.TechLayers) == 0 && len(a.BizDomains) == 0
}

func markVisitedLeaves(tree *DirectoryTreeV1) {
	if tree == nil {
		return
	}
	hasChild := buildChildrenIndex(tree)
	for i := range tree.Nodes {
		n := &tree.Nodes[i]
		if len(hasChild[n.ID]) > 0 {
			continue
		}
		if n.Analysis == nil {
			continue
		}
		if n.Analysis.BfsControl != BfsControlContinue {
			continue
		}
		n.Analysis.BfsControl = BfsControlLeaf
	}
}

func buildChildrenIndex(tree *DirectoryTreeV1) map[string][]string {
	out := make(map[string][]string)
	if tree == nil {
		return out
	}
	for _, n := range tree.Nodes {
		if n.ParentID != "" {
			out[n.ParentID] = append(out[n.ParentID], n.ID)
		}
	}
	return out
}

func isVendoredJavaPath(rel string, projCtx *ProjectContextSummaryV1) bool {
	rel = filepath.ToSlash(strings.ToLower(rel))
	if !strings.Contains(rel, "/src/main/java/") {
		return false
	}
	idx := strings.Index(rel, "/src/main/java/")
	if idx < 0 {
		return false
	}
	pkgPath := rel[idx+len("/src/main/java/"):]
	for _, prefix := range vendoredJavaPathPrefixes {
		if strings.HasPrefix("/"+pkgPath, prefix) {
			if projCtx != nil {
				pkgHint := strings.ReplaceAll(pkgPath, "/", ".")
				for _, root := range projCtx.FirstPartyBoundary.PackageRoots {
					root = strings.ToLower(strings.TrimSpace(root))
					if root != "" && (pkgHint == root || strings.HasPrefix(pkgHint, root+".")) {
						return false
					}
				}
			}
			return true
		}
	}
	return false
}

func isFirstPartyJavaPath(rel string, projCtx *ProjectContextSummaryV1) bool {
	rel = filepath.ToSlash(rel)
	if !strings.Contains(rel, "/src/main/java/") {
		return false
	}
	if isVendoredJavaPath(rel, projCtx) {
		return false
	}
	for _, suffix := range firstPartyBootstrapPathSuffixes {
		if rel == strings.TrimPrefix(suffix, "/") || strings.HasSuffix(rel, suffix) || strings.Contains(rel, suffix+"/") {
			return true
		}
	}
	if projCtx != nil && projCtx.matchesFirstPartyPath(rel) {
		return true
	}
	return isLikelyFirstPartyContainerPath(rel)
}

func sanitizeDirAnalysis(node *DirectoryNode, projCtx *ProjectContextSummaryV1, codeRoot string, analysis *DirAnalysis) *DirAnalysis {
	if analysis == nil || node == nil {
		return analysis
	}
	out := *analysis
	if out.DepInfo != nil {
		out.DepInfo = &DepInfo{
			Name:        out.DepInfo.Name,
			Group:       out.DepInfo.Group,
			Version:     normalizeVersion(out.DepInfo.Version),
			Description: out.DepInfo.Description,
		}
	}

	rel := filepath.ToSlash(node.RelPath)
	firstPartyJava := isFirstPartyJavaPath(rel, projCtx)

	if firstPartyJava && out.BfsControl == BfsControlStop {
		out.BfsControl = BfsControlContinue
		out.IsBusiness = true
		out.IsHttpEntry = false
		out.DepInfo = nil
		if len(out.TechLayers) == 0 || (len(out.TechLayers) == 1 && out.TechLayers[0] == "tech:third_party") {
			out.TechLayers = []string{"tech:other"}
		}
	}

	if firstPartyJava {
		out.DepInfo = nil
		if out.BfsControl == "" {
			out.BfsControl = BfsControlContinue
		}
		if out.BfsControl != BfsControlStop {
			out.IsBusiness = true
		}
	}

	if out.BfsControl == BfsControlStop && !out.IsBusiness {
		if !shouldTreatAsThirdPartyDir(node, projCtx) && !isVendoredJavaPath(rel, projCtx) {
			out.BfsControl = BfsControlContinue
			out.IsBusiness = true
			out.DepInfo = nil
			if len(out.TechLayers) == 0 {
				out.TechLayers = []string{"tech:other"}
			}
		}
	}

	if out.BfsControl == BfsControlStop && !out.IsBusiness {
		out.DepInfo = enrichDepInfo(node, codeRoot, out.DepInfo)
		if out.DepInfo != nil && normalizeVersion(out.DepInfo.Version) == "" {
			out.DepInfo.Version = ""
		}
	}

	return &out
}

func dirAnalysisRejectsFirstPartyStop(node *DirectoryNode, projCtx *ProjectContextSummaryV1, analysis *DirAnalysis) bool {
	if node == nil || analysis == nil {
		return false
	}
	return isFirstPartyJavaPath(node.RelPath, projCtx) && analysis.BfsControl == BfsControlStop && !isVendoredJavaPath(node.RelPath, projCtx)
}
