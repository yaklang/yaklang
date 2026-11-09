package ssaapi

import (
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

// predecessorsFor returns the predecessor edges for nodeID. The second result
// is false when no snapshot is authoritative (empty result or nil snapshot),
// so the caller can fall back to the per-node reader for legacy rows.
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

// getAuditEdgeSnapshot keeps one result snapshot per Program. Report
// generation processes one result at a time, so a single slot bounds memory
// without a per-result cache that could grow with the number of rules.
func (p *Program) getAuditEdgeSnapshot(resultID uint) *auditEdgeSnapshot {
	if p == nil || resultID == 0 {
		return nil
	}
	p.auditEdgeMu.Lock()
	defer p.auditEdgeMu.Unlock()
	if p.auditEdgeResultID == resultID && p.auditEdgeSnapshot != nil {
		return p.auditEdgeSnapshot
	}
	if !p.auditEdgeChecked {
		p.auditEdgeChecked = true
		p.auditEdgeLegacy = ssadb.HasAuditEdgesWithoutResult(ssadb.GetDB())
	}
	if p.auditEdgeLegacy {
		// Legacy rows may carry result_id = 0 while their nodes claim a real
		// result. A result-scoped query would silently drop them, so keep the
		// per-node reader for this program.
		return nil
	}
	edges, err := ssadb.LoadAuditEdgeTargetsByResultID(ssadb.GetDB(), resultID)
	if err != nil {
		log.Errorf("load audit edges for result %d failed: %v", resultID, err)
		return nil
	}
	snapshot := buildAuditEdgeSnapshot(edges)
	p.auditEdgeResultID = resultID
	p.auditEdgeSnapshot = snapshot
	return snapshot
}
