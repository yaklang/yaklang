package ssaapi

import (
	"time"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
)

// auditEdgeSnapshot is the in-memory form of one audit result's edges. Report
// generation visits many nodes of the same result; loading the result once
// turns the per-node predecessor/depend/effect queries into one query.
type auditEdgeSnapshot struct {
	predecessors map[string][]ssadb.AuditPredecessor
	dependsOn    map[string][]string
	effectsOn    map[string][]string
	total        int
}

func buildAuditEdgeSnapshot(edges []ssadb.AuditEdgeTarget) *auditEdgeSnapshot {
	snapshot := &auditEdgeSnapshot{
		predecessors: make(map[string][]ssadb.AuditPredecessor),
		dependsOn:    make(map[string][]string),
		effectsOn:    make(map[string][]string),
		total:        len(edges),
	}
	for _, edge := range edges {
		if edge.FromNode == "" || edge.ToNode == "" {
			continue
		}
		switch edge.EdgeType {
		case ssadb.EdgeType_Predecessor:
			snapshot.predecessors[edge.FromNode] = append(snapshot.predecessors[edge.FromNode], ssadb.AuditPredecessor{
				ToNode: edge.ToNode,
				Step:   edge.AnalysisStep,
				Label:  edge.AnalysisLabel,
			})
		case ssadb.EdgeType_DependsOn:
			snapshot.dependsOn[edge.FromNode] = append(snapshot.dependsOn[edge.FromNode], edge.ToNode)
		case ssadb.EdgeType_EffectsOn:
			snapshot.effectsOn[edge.FromNode] = append(snapshot.effectsOn[edge.FromNode], edge.ToNode)
		}
	}
	return snapshot
}

// predecessorsFor returns the predecessor edges recorded for nodeID. A nil
// snapshot means no result-scoped view is available; the caller then keeps the
// per-node reader.
func (s *auditEdgeSnapshot) predecessorsFor(nodeID string) ([]ssadb.AuditPredecessor, bool) {
	if s == nil {
		return nil, false
	}
	return s.predecessors[nodeID], true
}

func (s *auditEdgeSnapshot) dependsOnFor(nodeID string) ([]string, bool) {
	if s == nil {
		return nil, false
	}
	return s.dependsOn[nodeID], true
}

func (s *auditEdgeSnapshot) effectsOnFor(nodeID string) ([]string, bool) {
	if s == nil {
		return nil, false
	}
	return s.effectsOn[nodeID], true
}

// auditEdgeSnapshotTTL bounds how long one result's edges stay resident. A
// Program serves many results during report generation; the TTL plus the
// capacity keeps a small working window without reloading on every revisit.
const (
	auditEdgeSnapshotTTL      = 10 * time.Minute
	auditEdgeSnapshotCapacity = 16
)

// getAuditEdgeSnapshot returns the result-scoped edge view, loading it once
// per result. Several rules walk results in different orders, so a single slot
// would drop an earlier result and reload it later; the cache keeps the
// recently visited results and GetOrLoad makes concurrent misses share one
// query.
//
// Nodes and edges are stamped with the same ResultId by the writer, so a
// result-scoped read sees exactly the edges the per-node reader would return
// for that result's nodes. An empty result means the graph was not persisted
// under this id (older writers stamped result_id = 0); return nil so the
// caller keeps the per-node reader instead of reporting "no edges".
func (p *Program) getAuditEdgeSnapshot(resultID uint) *auditEdgeSnapshot {
	if p == nil || resultID == 0 {
		return nil
	}
	snapshot, err := p.auditEdgeSnapshotCache().GetOrLoad(resultID, func() (*auditEdgeSnapshot, error) {
		edges, err := ssadb.LoadAuditEdgeTargetsByResultID(ssadb.GetDB(), resultID)
		if err != nil {
			return nil, err
		}
		if len(edges) == 0 {
			return nil, nil
		}
		return buildAuditEdgeSnapshot(edges), nil
	})
	if err != nil {
		log.Errorf("load audit edges for result %d failed: %v", resultID, err)
		return nil
	}
	return snapshot
}

// auditEdgeSnapshotCache returns the per-result snapshot cache, building it on
// first use. The cache is internally locked, so only construction needs the
// once.
func (p *Program) auditEdgeSnapshotCache() *utils.CacheExWithKey[uint, *auditEdgeSnapshot] {
	p.auditEdgeCacheOnce.Do(func() {
		p.auditEdgeSnapshots = utils.NewCacheExWithKey[uint, *auditEdgeSnapshot](
			utils.WithCacheTTL(auditEdgeSnapshotTTL),
			utils.WithCacheCapacity(auditEdgeSnapshotCapacity),
		)
	})
	return p.auditEdgeSnapshots
}

// resetAuditEdgeSnapshot drops the cached result snapshots. It is used when
// the program's result set changes.
func (p *Program) resetAuditEdgeSnapshot() {
	if p == nil {
		return
	}
	// Purge on a cache that was never built only builds an empty one.
	p.auditEdgeSnapshotCache().Purge()
}
