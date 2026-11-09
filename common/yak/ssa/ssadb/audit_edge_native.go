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
		var toNode sql.NullString
		if err := rows.Scan(&toNode); err != nil {
			RecordDBRead(time.Since(started), true)
			logNativeSQL(query, time.Since(started), err)
			return out
		}
		if toNode.Valid {
			out = append(out, toNode.String)
		}
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
			createdAt sql.NullTime
			updatedAt sql.NullTime
			deletedAt sql.NullTime
			taskID    sql.NullString
			resultID  sql.NullInt64
			fromNode  sql.NullString
			toNode    sql.NullString
			program   sql.NullString
			riskHash  sql.NullString
			edgeType  sql.NullString
			step      sql.NullInt64
			stepLabel sql.NullString
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
				CreatedAt: createdAt.Time,
				UpdatedAt: updatedAt.Time,
			},
			TaskId:          taskID.String,
			ResultId:        uint(resultID.Int64),
			FromNode:        fromNode.String,
			ToNode:          toNode.String,
			ProgramName:     program.String,
			RiskFeatureHash: riskHash.String,
			EdgeType:        AuditEdgeType(edgeType.String),
			AnalysisStep:    step.Int64,
			AnalysisLabel:   stepLabel.String,
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

// AuditPredecessor is the projection GetPredecessors needs from a predecessor
// edge. Reading only these columns avoids materializing the wide AuditEdge for
// every step in a dataflow report.
type AuditPredecessor struct {
	ToNode string
	Step   int64
	Label  string
}

func nativePredecessorTargets(db *gorm.DB, fromID string) []AuditPredecessor {
	if db == nil {
		db = GetDB()
	}
	if db == nil || fromID == "" {
		return nil
	}

	ctx, cancel := nativeQueryContext(context.Background())
	defer cancel()
	query := bindSQLPlaceholdersDB(db,
		`SELECT to_node, analysis_step, analysis_label FROM "`+TableAuditEdges+`" `+
			`WHERE from_node = ? AND edge_type = ? AND deleted_at IS NULL`)
	started := time.Now()
	rows, err := nativeQueryContextDB(db, ctx, query, fromID, string(EdgeType_Predecessor))
	if err != nil {
		RecordDBRead(time.Since(started), true)
		logNativeSQL(query, time.Since(started), err)
		return nil
	}
	defer rows.Close()

	var out []AuditPredecessor
	for rows.Next() {
		var (
			toNode sql.NullString
			step   sql.NullInt64
			label  sql.NullString
		)
		if err := rows.Scan(&toNode, &step, &label); err != nil {
			RecordDBRead(time.Since(started), true)
			logNativeSQL(query, time.Since(started), err)
			return out
		}
		if !toNode.Valid {
			continue
		}
		out = append(out, AuditPredecessor{
			ToNode: toNode.String,
			Step:   step.Int64,
			Label:  label.String,
		})
	}
	if err := rows.Err(); err != nil {
		RecordDBRead(time.Since(started), true)
		logNativeSQL(query, time.Since(started), err)
		return out
	}
	RecordDBRead(time.Since(started), false)
	return out
}

func GetPredecessorTargetsByFromID(db *gorm.DB, fromID string) []AuditPredecessor {
	return nativePredecessorTargets(db, fromID)
}

// AuditEdgeTarget is the narrow projection used when a whole audit result is
// loaded at once. The dataflow report walks one result's graph and would
// otherwise issue one predecessor/depend/effect query per node.
type AuditEdgeTarget struct {
	FromNode      string
	ToNode        string
	EdgeType      AuditEdgeType
	AnalysisStep  int64
	AnalysisLabel string
}

type auditEdgeTargetRow struct {
	FromNode      sql.NullString
	ToNode        sql.NullString
	EdgeType      sql.NullString
	AnalysisStep  sql.NullInt64
	AnalysisLabel sql.NullString
}

// LoadAuditEdgeTargetsByResultID loads every live edge of one audit result in
// a single query. GORM keeps the bind variables and the soft-delete predicate
// explicit; the projection avoids materializing the wide AuditEdge model. The
// GORM query callback records the op stats, so this function does not call
// RecordDBRead itself.
//
// The caller caches the result because report generation visits many nodes of
// the same result. NULL columns are mapped to zero values, matching the
// per-node native readers.
func LoadAuditEdgeTargetsByResultID(db *gorm.DB, resultID uint) ([]AuditEdgeTarget, error) {
	if db == nil {
		db = GetDB()
	}
	if db == nil || resultID == 0 {
		return nil, nil
	}

	var rows []auditEdgeTargetRow
	err := db.Table(TableAuditEdges).
		Select("from_node, to_node, edge_type, analysis_step, analysis_label").
		Where("result_id = ?", resultID).
		Where("deleted_at IS NULL").
		Find(&rows).Error
	if err != nil {
		return nil, err
	}

	out := make([]AuditEdgeTarget, 0, len(rows))
	for _, row := range rows {
		if !row.FromNode.Valid || !row.ToNode.Valid {
			continue
		}
		out = append(out, AuditEdgeTarget{
			FromNode:      row.FromNode.String,
			ToNode:        row.ToNode.String,
			EdgeType:      AuditEdgeType(row.EdgeType.String),
			AnalysisStep:  row.AnalysisStep.Int64,
			AnalysisLabel: row.AnalysisLabel.String,
		})
	}
	return out, nil
}
