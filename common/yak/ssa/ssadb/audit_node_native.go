package ssadb

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"github.com/yaklang/gorm"
)

const auditNodeNativeColumns = `id, created_at, updated_at, deleted_at, task_id, result_id, ` +
	`result_variable, result_index, risk_hash, risk_feature_hash, rule_name, rule_title, ` +
	`program_name, is_entry_node, ir_code_id, node_id, tmp_value, tmp_value_file_hash, ` +
	`tmp_start_offset, tmp_end_offset, verbose_name`

// scanAuditNode decodes the fixed column order used by auditNodeNativeColumns.
// NULL is mapped to the zero value, which is what GORM does for these fields;
// old imported reports can contain NULLs in any of the string/number columns.
func scanAuditNode(rows *sql.Rows) (*AuditNode, error) {
	var (
		rowID          uint
		createdAt      sql.NullTime
		updatedAt      sql.NullTime
		deletedAt      sql.NullTime
		taskID         sql.NullString
		resultID       sql.NullInt64
		resultVariable sql.NullString
		resultIndex    sql.NullInt64
		riskHash       sql.NullString
		riskFeature    sql.NullString
		ruleName       sql.NullString
		ruleTitle      sql.NullString
		programName    sql.NullString
		isEntryNode    sql.NullBool
		irCodeID       sql.NullInt64
		nodeID         sql.NullString
		tmpValue       sql.NullString
		tmpFileHash    sql.NullString
		tmpStart       sql.NullInt64
		tmpEnd         sql.NullInt64
		verboseName    sql.NullString
	)
	if err := rows.Scan(
		&rowID, &createdAt, &updatedAt, &deletedAt, &taskID, &resultID,
		&resultVariable, &resultIndex, &riskHash, &riskFeature, &ruleName,
		&ruleTitle, &programName, &isEntryNode, &irCodeID, &nodeID, &tmpValue,
		&tmpFileHash, &tmpStart, &tmpEnd, &verboseName,
	); err != nil {
		return nil, err
	}
	node := &AuditNode{
		Model: gorm.Model{
			ID:        rowID,
			CreatedAt: createdAt.Time,
			UpdatedAt: updatedAt.Time,
		},
		AuditNodeStatus: AuditNodeStatus{
			TaskId:          taskID.String,
			ResultId:        uint(resultID.Int64),
			ResultVariable:  resultVariable.String,
			ResultIndex:     uint(resultIndex.Int64),
			RiskHash:        riskHash.String,
			RiskFeatureHash: riskFeature.String,
			RuleName:        ruleName.String,
			RuleTitle:       ruleTitle.String,
			ProgramName:     programName.String,
		},
		IsEntryNode:      isEntryNode.Bool,
		IRCodeID:         irCodeID.Int64,
		NodeID:           nodeID.String,
		TmpValue:         tmpValue.String,
		TmpValueFileHash: tmpFileHash.String,
		TmpStartOffset:   int(tmpStart.Int64),
		TmpEndOffset:     int(tmpEnd.Int64),
		VerboseName:      verboseName.String,
	}
	if deletedAt.Valid {
		deleted := deletedAt.Time
		node.DeletedAt = &deleted
	}
	return node, nil
}

// nativeGetAuditNodeById is on the dataflow-report read path: every
// predecessor target is materialized through it while a report with dataflow
// paths is opened. GORM's First() builds a scope, selects every model field
// through reflection, and allocates the query chain for each node. Read the
// fixed column set directly; the table is tiny compared with ir_codes, so the
// win is allocation and CPU, not SQL planning.
func nativeGetAuditNodeById(db *gorm.DB, id string) (*AuditNode, error) {
	if db == nil {
		db = GetDB()
	}
	if db == nil || id == "" {
		return nil, gorm.ErrRecordNotFound
	}

	ctx, cancel := nativeQueryContext(context.Background())
	defer cancel()
	query := bindSQLPlaceholdersDB(db,
		`SELECT `+auditNodeNativeColumns+` FROM "`+TableAuditNodes+`" `+
			`WHERE node_id = ? AND deleted_at IS NULL ORDER BY id LIMIT 1`)
	started := time.Now()
	rows, err := nativeQueryContextDB(db, ctx, query, id)
	if err != nil {
		RecordDBRead(time.Since(started), true)
		logNativeSQL(query, time.Since(started), err)
		return nil, err
	}
	defer rows.Close()
	if !rows.Next() {
		err := rows.Err()
		if err == nil {
			err = gorm.ErrRecordNotFound
		}
		RecordDBRead(time.Since(started), err != nil)
		return nil, err
	}
	node, err := scanAuditNode(rows)
	if err != nil {
		RecordDBRead(time.Since(started), true)
		logNativeSQL(query, time.Since(started), err)
		return nil, err
	}
	if err := rows.Err(); err != nil {
		RecordDBRead(time.Since(started), true)
		logNativeSQL(query, time.Since(started), err)
		return nil, err
	}
	RecordDBRead(time.Since(started), false)
	return node, nil
}

// nativeGetAuditNodesByIds is the batch form used when a report was produced
// from an in-memory program. One query replaces one GORM First() per node.
func nativeGetAuditNodesByIds(db *gorm.DB, ids []string) ([]*AuditNode, error) {
	if db == nil {
		db = GetDB()
	}
	if db == nil || len(ids) == 0 {
		return nil, nil
	}

	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	query := bindSQLPlaceholdersDB(db,
		`SELECT `+auditNodeNativeColumns+` FROM "`+TableAuditNodes+`" `+
			`WHERE node_id IN (`+placeholders+`) AND deleted_at IS NULL`)
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	ctx, cancel := nativeQueryContext(context.Background())
	defer cancel()
	started := time.Now()
	rows, err := nativeQueryContextDB(db, ctx, query, args...)
	if err != nil {
		RecordDBRead(time.Since(started), true)
		logNativeSQL(query, time.Since(started), err)
		return nil, err
	}
	defer rows.Close()

	var out []*AuditNode
	for rows.Next() {
		node, err := scanAuditNode(rows)
		if err != nil {
			RecordDBRead(time.Since(started), true)
			logNativeSQL(query, time.Since(started), err)
			return out, err
		}
		out = append(out, node)
	}
	if err := rows.Err(); err != nil {
		RecordDBRead(time.Since(started), true)
		logNativeSQL(query, time.Since(started), err)
		return out, err
	}
	RecordDBRead(time.Since(started), false)
	return out, nil
}
