package loop_ssa_api_discovery

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
)

// EstimateWorkUnits converts directory analysis results into work units for Phase1 feature processing.
// It merges small directories, splits large ones, and computes priority.
func EstimateWorkUnits(tree *DirectoryTreeV1) ([]*WorkUnit, error) {
	if tree == nil {
		return nil, fmt.Errorf("nil directory tree")
	}

	// 1. Collect all business directories (is_business=true, non-bfs:stop)
	businessDirs := collectBusinessDirs(tree)
	if len(businessDirs) == 0 {
		return nil, nil
	}

	// 2. Split/merge by size
	units := splitAndMergeDirs(businessDirs)

	// 3. Compute priority for each
	for _, u := range units {
		u.Priority = computePriority(u)
	}

	// 4. Sort by priority (lower = higher priority)
	sort.Slice(units, func(i, j int) bool {
		if units[i].Priority != units[j].Priority {
			return units[i].Priority < units[j].Priority
		}
		return units[i].EstimatedKB > units[j].EstimatedKB // larger first within same priority
	})

	return units, nil
}

func collectBusinessDirs(tree *DirectoryTreeV1) []*DirectoryNode {
	var dirs []*DirectoryNode
	for i := range tree.Nodes {
		n := &tree.Nodes[i]
		if n.Analysis == nil {
			continue
		}
		if n.Analysis.BfsControl == BfsControlStop {
			continue
		}
		if !n.Analysis.IsBusiness {
			continue
		}
		dirs = append(dirs, n)
	}
	return dirs
}

func splitAndMergeDirs(dirs []*DirectoryNode) []*WorkUnit {
	if len(dirs) == 0 {
		return nil
	}

	// Group by parent
	parentGroups := make(map[string][]*DirectoryNode)
	var rootDirs []*DirectoryNode
	for _, d := range dirs {
		if d.ParentID == "" || d.RelPath == "" {
			rootDirs = append(rootDirs, d)
			continue
		}
		parentGroups[d.ParentID] = append(parentGroups[d.ParentID], d)
	}

	var units []*WorkUnit

	// Process each parent's children as a group
	for _, children := range parentGroups {
		merged := mergeSmallDirs(children)
		units = append(units, merged...)
	}

	// Handle root-level dirs (no parent)
	for _, d := range rootDirs {
		units = append(units, dirToWorkUnit(d))
	}

	return units
}

func mergeSmallDirs(dirs []*DirectoryNode) []*WorkUnit {
	if len(dirs) == 0 {
		return nil
	}

	var large, small []*DirectoryNode
	for _, d := range dirs {
		if d.TotalSizeKB >= WorkUnitKB {
			large = append(large, d)
		} else {
			small = append(small, d)
		}
	}

	var units []*WorkUnit

	// Large dirs become their own work unit
	for _, d := range large {
		units = append(units, dirToWorkUnit(d))
	}

	// Small dirs: group by parent and merge until >= WorkUnitKB
	if len(small) > 0 {
		merged := mergeSmallDirsIntoUnits(small)
		units = append(units, merged...)
	}

	return units
}

func mergeSmallDirsIntoUnits(dirs []*DirectoryNode) []*WorkUnit {
	if len(dirs) == 0 {
		return nil
	}

	// Sort by size descending
	sort.Slice(dirs, func(i, j int) bool {
		return dirs[i].TotalSizeKB > dirs[j].TotalSizeKB
	})

	var units []*WorkUnit
	var current *WorkUnit

	for _, d := range dirs {
		if current == nil {
			current = &WorkUnit{
				ID:          uuid.New().String(),
				Label:       "Merged: " + filepath.Base(d.RelPath),
				DirIDs:      []string{},
				EntryFiles:  []string{},
				TechLayers:  []string{},
				BizDomains:  []string{},
				DbFeatures:  []string{},
				EstimatedKB: 0,
			}
		}

		if current.EstimatedKB+d.TotalSizeKB <= WorkUnitKB*2 {
			// Add to current unit
			current.DirIDs = append(current.DirIDs, d.ID)
			current.EstimatedKB += d.TotalSizeKB
			if d.Analysis != nil {
			if current.Label == "" {
				current.Label = "Merged: " + d.RelPath
			}
				if current.Description == "" {
					current.Description = d.Analysis.FunctionDesc
				}
				current.TechLayers = unionStringSlice(current.TechLayers, d.Analysis.TechLayers)
				current.BizDomains = unionStringSlice(current.BizDomains, d.Analysis.BizDomains)
				current.DbFeatures = unionStringSlice(current.DbFeatures, d.Analysis.DbFeatures)
				if d.Analysis.IsHttpEntry {
					current.EntryFiles = unionStringSlice(current.EntryFiles, d.Analysis.HttpEntryFiles)
					current.SurfaceKind = SurfaceKindHTTPAPI
				}
			}
		} else {
			// Start new unit
			units = append(units, current)
			current = &WorkUnit{
				ID:          uuid.New().String(),
				Label:       "Merged: " + d.RelPath,
				DirIDs:      []string{d.ID},
				EntryFiles:  []string{},
				TechLayers:  []string{},
				BizDomains:  []string{},
				DbFeatures:  []string{},
				EstimatedKB: d.TotalSizeKB,
			}
			if d.Analysis != nil {
				current.Description = d.Analysis.FunctionDesc
				current.TechLayers = append([]string(nil), d.Analysis.TechLayers...)
				current.BizDomains = append([]string(nil), d.Analysis.BizDomains...)
				current.DbFeatures = append([]string(nil), d.Analysis.DbFeatures...)
				if d.Analysis.IsHttpEntry {
					current.EntryFiles = append([]string(nil), d.Analysis.HttpEntryFiles...)
					current.SurfaceKind = SurfaceKindHTTPAPI
				}
			}
		}
	}

	if current != nil {
		units = append(units, current)
	}

	return units
}

func dirToWorkUnit(n *DirectoryNode) *WorkUnit {
	u := &WorkUnit{
		ID:          uuid.New().String(),
		Label:       n.RelPath,
		Description: "",
		DirIDs:      []string{n.ID},
		EntryFiles:  []string{},
		EstimatedKB: n.TotalSizeKB,
		TechLayers:  []string{},
		BizDomains:  []string{},
		DbFeatures:  []string{},
	}

	if n.Analysis != nil {
		if n.Analysis.FunctionDesc != "" {
			u.Description = n.Analysis.FunctionDesc
		}
		u.TechLayers = append([]string(nil), n.Analysis.TechLayers...)
		u.BizDomains = append([]string(nil), n.Analysis.BizDomains...)
		u.DbFeatures = append([]string(nil), n.Analysis.DbFeatures...)
		if n.Analysis.IsHttpEntry {
			u.EntryFiles = append([]string(nil), n.Analysis.HttpEntryFiles...)
			u.SurfaceKind = SurfaceKindHTTPAPI
		}
	}

	if u.SurfaceKind == "" {
		u.SurfaceKind = SurfaceKindCodeOnly
	}

	return u
}

// computePriority returns a priority integer (lower = higher priority).
// Priority: API+DB(1) > API no DB(2) > DB no API(3) > API(4) > Other(5)
func computePriority(u *WorkUnit) int {
	isHttpEntry := len(u.EntryFiles) > 0
	hasDB := containsAny(u.DbFeatures, "db:jpa", "db:mybatis", "db:sql", "db:cache")
	hasApi := isHttpEntry

	if hasApi && hasDB {
		return 1
	}
	if hasApi && !hasDB {
		return 2
	}
	if hasDB && !hasApi {
		return 3
	}
	if hasApi {
		return 4
	}
	return 5
}

func containsAny(slice []string, vals ...string) bool {
	for _, s := range slice {
		for _, v := range vals {
			if s == v {
				return true
			}
		}
	}
	return false
}

func unionStringSlice(a, b []string) []string {
	set := make(map[string]bool)
	var result []string
	for _, v := range a {
		if !set[v] {
			set[v] = true
			result = append(result, v)
		}
	}
	for _, v := range b {
		if !set[v] {
			set[v] = true
			result = append(result, v)
		}
	}
	return result
}

// WorkUnitWithParentID extends WorkUnit with parent context.
type WorkUnitWithParentID struct {
	*WorkUnit
	ParentDirID string `json:"parent_dir_id,omitempty"`
}

// BuildFeatureInventoryFromWorkUnits generates a FeatureInventoryV1 from work units.
// This replaces the F1 Agent output while maintaining compatibility with downstream phases.
func BuildFeatureInventoryFromWorkUnits(units []*WorkUnit) *FeatureInventoryV1 {
	inv := &FeatureInventoryV1{
		SchemaVersion: artifactV2SchemaVersion,
		GeneratedAt:   time.Now().UTC().Format("2006-01-02T15:04:05Z"),
		Features:      []FeatureInventoryEntry{},
		Coverage: FeatureCoverageResult{
			Policy:        "directory_analysis_work_units",
			TotalRequired: len(units),
			Covered:       len(units),
			Complete:      true,
		},
	}

	for _, u := range units {
		entry := FeatureInventoryEntry{
			FeatureID:   u.FeatureID,
			Label:       u.Label,
			Description: u.Description,
			PackagePatterns: u.DirIDs,
			SurfaceKind: u.SurfaceKind,
			EntryFiles:  u.EntryFiles,
		}
		if entry.FeatureID == "" {
			entry.FeatureID = u.ID
		}
		inv.Features = append(inv.Features, entry)
	}

	return inv
}

// DeriveFeatureID generates a feature_id from the work unit label.
func DeriveFeatureID(label string) string {
	id := strings.ToLower(label)
	id = strings.ReplaceAll(id, " ", "_")
	id = strings.ReplaceAll(id, "/", "_")
	id = strings.ReplaceAll(id, "-", "_")
	// Remove non-alphanumeric except underscore
	var clean []rune
	for _, r := range id {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' {
			clean = append(clean, r)
		}
	}
	return string(clean)
}

func deriveDescription(domainSeg string, bizDomains []string) string {
	switch domainSeg {
	case "controller":
		if len(bizDomains) > 0 {
			return bizDomains[0] + " HTTP接口层"
		}
		return "HTTP接口控制器"
	case "service":
		return "业务服务层"
	case "dao":
		return "数据访问层"
	case "entities":
		return "数据实体模型"
	case "config":
		return "配置与初始化"
	case "interceptor", "filter":
		return "请求拦截器"
	case "common", "base":
		return "公共基础组件"
	default:
		return domainSeg + " 功能模块"
	}
}

// PersistFeatureInventory persists work units as both the raw units JSON and feature_inventory.json.
func PersistFeatureInventory(rt *Runtime, units []*WorkUnit) error {
	if rt == nil {
		return nil
	}

	// Assign feature IDs
	for _, u := range units {
		if u.FeatureID == "" {
			u.FeatureID = DeriveFeatureID(u.Label)
		}
	}

	// Write work units JSON
	unitsPath := filepath.Join(rt.WorkDir, store.SubDirName(), "work_units.json")
	b, _ := json.MarshalIndent(units, "", "  ")
	if err := writeJSONFile(unitsPath, b); err != nil {
		return err
	}

	// Write feature_inventory.json
	inv := BuildFeatureInventoryFromWorkUnits(units)
	if err := persistFeatureInventory(rt, inv); err != nil {
		return err
	}

	return nil
}
