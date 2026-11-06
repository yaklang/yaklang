package ssadb

import (
	"context"
	"database/sql"
	"time"

	"github.com/yaklang/gorm"
)

// Audit-edge reads are on the dataflow traversal hot path: every node asks for
// its predecessor / depend-on / effect-on edges. GORM builds a full query
// chain (Scope.Fields, AddToVars, clone) for each call; on large projects that
// dominated both allocation and SQLite CPU. These native reads bind the few
// columns the callers actually consume.
//
// The soft-delete predicate mirrors GORM's default `deleted_at IS NULL`.

func nativeAuditEdgeTargets(db *gorm.DB, fromID string, edgeType AuditEdgeType) []string {
	if db == nil {
		db = GetDB()
	}
	if db == nil || fromID == "" {
		return nil
	}

	ctx, cancel := nativeQueryContext(context.Background())
	defer cancel()
	query := bindSQLPlaceholdersDB(db,
		`SELECT to_node FROM "`+TableAuditEdges+`" WHERE from_node = ? AND edge_type = ? AND deleted_at IS NULL`)
	started := time.Now()
	rows, err := nativeQueryContextDB(db, ctx, query, fromID, string(edgeType))
	if err != nil {
		RecordDBRead(time.Since(started), true)
		logNativeSQL(query, time.Since(started), err)
		return nil
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var toNode string
		if err := rows.Scan(&toNode); err != nil {
			RecordDBRead(time.Since(started), true)
			logNativeSQL(query, time.Since(started), err)
			return out
		}
		out = append(out, toNode)
	}
	if err := rows.Err(); err != nil {
		RecordDBRead(time.Since(started), true)
		logNativeSQL(query, time.Since(started), err)
		return out
	}
	RecordDBRead(time.Since(started), false)
	return out
}

func nativePredecessorEdges(db *gorm.DB, fromID string) []*AuditEdge {
	if db == nil {
		db = GetDB()
	}
	if db == nil || fromID == "" {
		return nil
	}

	ctx, cancel := nativeQueryContext(context.Background())
	defer cancel()
	query := bindSQLPlaceholdersDB(db,
		`SELECT id, created_at, updated_at, deleted_at, task_id, result_id, from_node, to_node, `+
			`program_name, risk_feature_hash, edge_type, analysis_step, analysis_label `+
			`FROM "`+TableAuditEdges+`" WHERE from_node = ? AND edge_type = ? AND deleted_at IS NULL`)
	started := time.Now()
	rows, err := nativeQueryContextDB(db, ctx, query, fromID, string(EdgeType_Predecessor))
	if err != nil {
		RecordDBRead(time.Since(started), true)
		logNativeSQL(query, time.Since(started), err)
		return nil
	}
	defer rows.Close()

	var out []*AuditEdge
	for rows.Next() {
		var (
			id        uint
			createdAt time.Time
			updatedAt time.Time
			deletedAt sql.NullTime
			taskID    string
			resultID  uint
			fromNode  string
			toNode    string
			program   string
			riskHash  string
			edgeType  string
			step      int64
			stepLabel string
		)
		if err := rows.Scan(
			&id, &createdAt, &updatedAt, &deletedAt, &taskID, &resultID,
			&fromNode, &toNode, &program, &riskHash, &edgeType, &step, &stepLabel,
		); err != nil {
			RecordDBRead(time.Since(started), true)
			logNativeSQL(query, time.Since(started), err)
			return out
		}
		edge := &AuditEdge{
			Model: gorm.Model{
				ID:        id,
				CreatedAt: createdAt,
				UpdatedAt: updatedAt,
			},
			TaskId:          taskID,
			ResultId:        resultID,
			FromNode:        fromNode,
			ToNode:          toNode,
			ProgramName:     program,
			RiskFeatureHash: riskHash,
			EdgeType:        AuditEdgeType(edgeType),
			AnalysisStep:    step,
			AnalysisLabel:   stepLabel,
		}
		if deletedAt.Valid {
			deleted := deletedAt.Time
			edge.DeletedAt = &deleted
		}
		out = append(out, edge)
	}
	if err := rows.Err(); err != nil {
		RecordDBRead(time.Since(started), true)
		logNativeSQL(query, time.Since(started), err)
		return out
	}
	RecordDBRead(time.Since(started), false)
	return out
}
