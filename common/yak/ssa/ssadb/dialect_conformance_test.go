package ssadb

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/consts"
)

// TestSSADialectConformance runs the SSA store's dialect-sensitive statements
// against every backend. SQLite runs locally; PostgreSQL and MySQL run when
// their DSN environment variable is set, which the ssa-sql-dialect workflow
// does with service containers. DM additionally needs -tags dameng and an
// isolated fixture, so its subtest skips unless YAK_SSA_CONFORMANCE_DM_DSN is
// supplied.
func TestSSADialectConformance(t *testing.T) {
	tests := []struct {
		backend string
		env     string
		dialect sqlDialect
	}{
		{backend: "sqlite", dialect: sqlDialectSQLite},
		{backend: "postgres", env: "YAK_SSA_CONFORMANCE_PG_DSN", dialect: sqlDialectPostgres},
		{backend: "mysql", env: "YAK_SSA_CONFORMANCE_MYSQL_DSN", dialect: sqlDialectMySQL},
		// DM needs the native driver (-tags dameng) and an isolated fixture; CI
		// leaves this env unset, so the subtest skips unless an operator supplies it.
		{backend: "dm", env: "YAK_SSA_CONFORMANCE_DM_DSN", dialect: sqlDialectDM},
	}
	for _, tt := range tests {
		t.Run(tt.backend, func(t *testing.T) {
			dsn := conformanceDSN(t, tt.backend, tt.env)
			db := openConformanceDB(t, dsn)
			require.Equal(t, tt.dialect, dialectOf(db))
			runSSAConformance(t, db, tt.backend)
		})
	}
}

func conformanceDSN(t *testing.T, backend, env string) string {
	t.Helper()
	if backend == "sqlite" {
		return "sqlite://" + filepath.ToSlash(filepath.Join(t.TempDir(), "conformance.db"))
	}
	dsn := ""
	if env != "" {
		dsn = os.Getenv(env)
	}
	if dsn == "" {
		t.Skipf("set %s to run the %s conformance test", env, backend)
	}
	return dsn
}

func openConformanceDB(t *testing.T, dsn string) *gorm.DB {
	t.Helper()
	db, err := consts.CreateSSAProjectDatabaseRaw(dsn)
	require.NoError(t, err)
	previous := GetDB()
	SetDB(db)
	t.Cleanup(func() {
		SetDB(previous)
		require.NoError(t, db.Close())
	})
	return db
}

func runSSAConformance(t *testing.T, db *gorm.DB, backend string) {
	t.Helper()
	program := "dialect-conformance-" + uuid.NewString()
	t.Cleanup(func() {
		if err := DeleteProgramChecked(db, program); err != nil {
			t.Logf("cleanup program %s: %v", program, err)
		}
	})

	// The schema patches run on open; call them again to prove index
	// maintenance is idempotent on every dialect.
	patchIrCodeIndex(db)
	patchIrTypeIndexes(db)
	require.NoError(t, db.Error)
	indexSpecs := []IndexSpec{
		{Name: "idx_ir_codes_program_opcode", Table: TableIrCodes},
		{Name: "idx_ir_indices_program_value", Table: TableIrIndices},
		{Name: "idx_ir_name_pool_program_name_name", Table: TableIrNamePool},
		{Name: "ux_ir_codes_program_code", Table: TableIrCodes},
		{Name: "ux_ir_offsets_program_value_file_range", Table: TableIrOffsets},
		{Name: irTypeUniqueIndex, Table: TableIrTypes},
	}
	if backend != "dm" {
		// installDMIRIndexes deliberately does not manage the const-pool
		// indexes; DM keeps those tables unindexed until a dialect-specific
		// maintenance path exists.
		indexSpecs = append(indexSpecs,
			IndexSpec{Name: irConstPoolProgramIDIndex, Table: TableIrConstPool},
			IndexSpec{Name: irConstPoolSizeIndex, Table: TableIrConstPool},
			IndexSpec{Name: irCodeConstIDIndex, Table: TableIrCodes},
		)
	}
	for _, spec := range indexSpecs {
		require.True(t, hasNamedIndex(db, spec.Table, spec.Name),
			"%s: index %s on %s was not created", backend, spec.Name, spec.Table)
	}

	// Every dialect resolves column types through tableColumnType; a direct
	// information_schema query here would let the catalog helper drift.
	dataType, ok := tableColumnType(db, TableIrSources, "quoted_code")
	require.True(t, ok, "%s: tableColumnType could not find %s.quoted_code", backend, TableIrSources)
	require.NotEmpty(t, dataType)
	if backend == "mysql" {
		// patchIrSourceQuotedCode logs a warning instead of failing the open;
		// assert the column type so the ALTER TABLE is actually covered.
		require.Equal(t, "longtext", dataType)
	}

	if backend == "sqlite" || backend == "postgres" {
		exerciseOffsetsIndexUpgrade(t, db)
		exerciseRedundantIndexPrune(t, db)
	}

	require.NoError(t, db.Create(&IrProgram{
		ProgramName: program,
		ProgramKind: Application,
	}).Error)

	// Const pool: intern a value longer than IrCodeInlineConstStringLimit so the
	// search cannot fall back to ir_codes.string, then verify the pool probe and
	// both read paths. A long value also misses the short-const preload cache, so
	// GetIrConstValue must execute its single-row query.
	const textPrefix = "conformance-const-"
	text := textPrefix + strings.Repeat("x", IrCodeInlineConstStringLimit)
	constID := InternIrConst(program, IrConstKindString, text)
	require.NotZero(t, constID)
	require.NoError(t, GetIrConstPool(program).Flush(db))
	require.True(t, irConstPoolHasProgram(context.Background(), db, program),
		"%s: const pool probe must use dialect placeholders", backend)
	value := GetIrConstValue(db, program, constID)
	require.NotNil(t, value)
	require.Equal(t, text, value.Text)

	// IR upsert: the second write must replace the row, not duplicate it.
	code := &IrCode{
		ProgramName: program,
		CodeID:      1,
		Opcode:      irCodeConstOpcode,
		ConstType:   irCodeConstTypeNormal,
		ConstID:     constID,
		Name:        "original",
		// Deliberately empty: the const search must find this row through
		// ir_const_pool_v1, not the legacy ir_codes.string fallback.
		String: "",
	}
	require.NoError(t, SaveIrCodeBatch(db, []*IrCode{code}))
	code.Name = "updated"
	require.NoError(t, SaveIrCodeBatch(db, []*IrCode{code}))

	var stored IrCode
	require.NoError(t, db.Where("program_name = ? AND code_id = ?", program, code.CodeID).First(&stored).Error)
	require.Equal(t, "updated", stored.Name)
	nativeStored := nativeGetIrCodeItemById(db, program, code.CodeID)
	require.NotNil(t, nativeStored)
	require.Equal(t, "updated", nativeStored.Name)
	var codeCount int64
	require.NoError(t, db.Model(&IrCode{}).Where("program_name = ?", program).Count(&codeCount).Error)
	require.Equal(t, int64(1), codeCount)

	batchCodes, err := nativeGetIrCodesByIds(db, program, []int64{code.CodeID})
	require.NoError(t, err)
	require.Len(t, batchCodes, 1)
	require.Equal(t, "updated", batchCodes[0].Name)

	// Const search: exact and regexp both go through the pool query.
	ids, err := nativeGetIrCodeIDsByConstType(db, program, ExactCompare, text)
	require.NoError(t, err)
	require.Equal(t, []int64{code.CodeID}, ids)
	ids, err = nativeGetIrCodeIDsByConstType(db, program, RegexpCompare, "^conformance-const-.*$")
	require.NoError(t, err)
	require.Equal(t, []int64{code.CodeID}, ids)

	exerciseLegacyConstPath(t, db, program)

	// Native multi-row INSERT for SQLite/PostgreSQL; MySQL keeps the GORM path.
	require.NoError(t, InsertIrCodes(db, []*IrCode{{
		ProgramName: program,
		CodeID:      2,
		Opcode:      1,
		Name:        "native-insert",
	}}, 10))
	nativeInserted := nativeGetIrCodeItemById(db, program, 2)
	require.NotNil(t, nativeInserted)
	require.Equal(t, "native-insert", nativeInserted.Name)

	exerciseAuditNativeReads(t, db, program)

	// Offsets: the COALESCE expression index must accept a NULL/empty name.
	require.NoError(t, SaveIrOffsetBatch(db, []*IrOffset{{
		ProgramName: program,
		ValueID:     1,
		FileHash:    "conformance-hash",
		StartOffset: 10,
		EndOffset:   20,
	}}))
	var offsetCount int64
	require.NoError(t, db.Model(&IrOffset{}).Where("program_name = ?", program).Count(&offsetCount).Error)
	require.Equal(t, int64(1), offsetCount)

	// Types: upsert on (program_name, type_id).
	require.NoError(t, SaveIrTypeBatch(db, []*IrType{{ProgramName: program, TypeId: 1, String: "first"}}))
	require.NoError(t, SaveIrTypeBatch(db, []*IrType{{ProgramName: program, TypeId: 1, String: "second"}}))
	var storedType IrType
	require.NoError(t, db.Where("program_name = ? AND type_id = ?", program, 1).First(&storedType).Error)
	require.Equal(t, "second", storedType.String)
	nativeType := GetIrTypeItemById(db, program, 1)
	require.NotNil(t, nativeType)
	require.Equal(t, "second", nativeType.String)

	require.NoError(t, SaveIrIndexBatch(db, []*IrIndex{{ProgramName: program, ValueID: 1}}))

	// Name pool: reserveNamePoolID issues the quoted MAX(name_id) query.
	cache := NewNameCache(program, true)
	nameID := cache.GetID("conformance_name")
	require.NotZero(t, nameID)
	require.NoError(t, cache.Flush())
	var nameCount int64
	require.NoError(t, db.Model(&IrNamePool{}).Where("program_name = ?", program).Count(&nameCount).Error)
	require.Equal(t, int64(1), nameCount)

	// Program lookup moved into ssadb so PostgreSQL can use "~" instead of
	// the unsupported REGEXP operator.
	found, err := LoadProgramsByRegexp(db, "^"+regexp.QuoteMeta(program)+"$")
	require.NoError(t, err)
	require.Len(t, found, 1)
	require.Equal(t, program, found[0].ProgramName)

	exercisePublicReadAPIs(t, db, program, code.CodeID)

	exerciseProgramReplacement(t, db)

	require.NoError(t, DeleteProgramChecked(db, program))
	var remaining int64
	require.NoError(t, db.Model(&IrCode{}).Where("program_name = ?", program).Count(&remaining).Error)
	require.Zero(t, remaining)
}

// exercisePublicReadAPIs runs the exported read surface, not only the native
// helpers, so a dialect regression in the API layer is caught as well.
func exercisePublicReadAPIs(t *testing.T, db *gorm.DB, program string, codeID int64) {
	t.Helper()

	code := GetIrCodeById(db, program, codeID)
	require.NotNil(t, code, "GetIrCodeById")
	require.Equal(t, "updated", code.Name)
	require.NotNil(t, GetIrCodeByIdFast(db, program, codeID), "GetIrCodeByIdFast")
	PreloadIrCodesByIdsFast(db, program, []int64{codeID})
	require.NotNil(t, GetIrCodeByIdFast(db, program, codeID), "PreloadIrCodesByIdsFast")
	require.True(t, ExistsIrCodeById(db, program, codeID), "ExistsIrCodeById hit")
	require.False(t, ExistsIrCodeById(db, program, 999999), "ExistsIrCodeById miss")

	require.NotNil(t, GetIrTypeById(db, program, 1), "GetIrTypeById")

	apps := GetApplicationPrograms([]string{program, "missing-" + program})
	require.Contains(t, apps, program)
	require.NotContains(t, apps, "missing-"+program)
	app, err := GetApplicationProgram(program)
	require.NoError(t, err)
	require.Equal(t, program, app.ProgramName)

	require.Contains(t, AllProgramNames(db), program)
	var found bool
	for _, item := range AllPrograms(db) {
		if item.ProgramName == program {
			found = true
			break
		}
	}
	require.True(t, found, "AllPrograms")

	require.NoError(t, PreloadOffsetsByValueIDs(db, program, []int64{1}))
	offsets := GetOffsetByVariable("", 1, program)
	require.Len(t, offsets, 1)
	require.Equal(t, int64(1), offsets[0].ValueID)

	var searched int
	for item := range SearchIrCodeByOpcodes(db, context.Background(), program, irCodeConstOpcode) {
		if item != nil && item.CodeID == codeID {
			searched++
		}
	}
	require.Equal(t, 1, searched, "SearchIrCodeByOpcodes")

	source := &IrSource{
		ProgramName:    program,
		SourceCodeHash: "conformance-source-hash",
		FolderPath:     "/conformance/",
		FileName:       "main.java",
		QuotedCode:     "class Main {}",
	}
	require.NoError(t, db.Create(source).Error)
	sources, err := GetIrSourceByPath("/conformance")
	require.NoError(t, err)
	require.Len(t, sources, 1)
	storedSource, err := GetIrSourceByPathAndName("/conformance", "main.java")
	require.NoError(t, err)
	require.Equal(t, source.SourceCodeHash, storedSource.SourceCodeHash)

	tree, err := GetIrSourceTreeByProgram(program)
	require.NoError(t, err)
	require.Len(t, tree, 1)
	require.Equal(t, "main.java", tree[0].FileName)
	require.False(t, tree[0].IsDir)

	require.NoError(t, db.Create(&AuditResult{ProgramName: program, TaskID: "conformance-task"}).Error)
	count, err := CountAuditResults(db)
	require.NoError(t, err)
	require.Positive(t, count)
}

// exerciseProgramReplacement covers the transaction-scoped DELETE/UPDATE
// statements in program_replace.go. They run through nativeExec, so PostgreSQL
// placeholders and identifier quoting are exercised on every backend.
func exerciseProgramReplacement(t *testing.T, db *gorm.DB) {
	t.Helper()
	suffix := uuid.NewString()
	target := "replacement-target-" + suffix
	staged := "replacement-staged-" + suffix
	t.Cleanup(func() {
		_ = DeleteProgramChecked(db, target)
		_ = DeleteProgramChecked(db, staged)
	})

	require.NoError(t, db.Create(&IrProgram{ProgramName: target, ProgramKind: Application}).Error)
	require.NoError(t, db.Create(&IrProgram{ProgramName: staged, ProgramKind: Application}).Error)

	tx := db.Begin()
	require.NoError(t, tx.Error)
	require.NoError(t, deleteProgramRowsForReplacement(tx, target))
	require.NoError(t, renameProgramRowsForReplacement(tx, staged, target))
	require.NoError(t, tx.Commit().Error)

	var count int64
	require.NoError(t, db.Model(&IrProgram{}).Where("program_name = ?", target).Count(&count).Error)
	require.Equal(t, int64(1), count)
	require.NoError(t, db.Model(&IrProgram{}).Where("program_name = ?", staged).Count(&count).Error)
	require.Zero(t, count)
}

type legacyIrConstRow struct {
	ID          int64 `gorm:"primary_key;auto_increment"`
	ProgramName string
	CodeID      int64
	ConstHash   int64
	String      string `gorm:"type:text"`
}

func (legacyIrConstRow) TableName() string { return TableIrConsts }

// exerciseLegacyConstPath creates the table an older build wrote and verifies
// both exact and regexp lookups resolve through ir_consts_v1. A new database
// does not migrate this table, so the test owns its lifecycle.
func exerciseLegacyConstPath(t *testing.T, db *gorm.DB, program string) {
	t.Helper()
	if db.HasTable(TableIrConsts) {
		require.NoError(t, db.DropTable(&legacyIrConstRow{}).Error)
	}
	require.NoError(t, db.CreateTable(&legacyIrConstRow{}).Error)
	t.Cleanup(func() { _ = db.DropTable(&legacyIrConstRow{}).Error })

	legacyProgram := program + "-legacy"
	const legacyText = "legacy-const-value"
	require.NoError(t, db.Create(&legacyIrConstRow{
		ProgramName: legacyProgram,
		CodeID:      77,
		ConstHash:   IrConstHash(legacyText),
		String:      legacyText,
	}).Error)

	ids, err := nativeGetIrCodeIDsByConstType(db, legacyProgram, ExactCompare, legacyText)
	require.NoError(t, err)
	require.Equal(t, []int64{77}, ids)
	ids, err = nativeGetIrCodeIDsByConstType(db, legacyProgram, RegexpCompare, "^legacy-const-.*$")
	require.NoError(t, err)
	require.Equal(t, []int64{77}, ids)
}

// exerciseOffsetsIndexUpgrade starts from the pre-COALESCE index definition and
// proves both SQLite and PostgreSQL rewrite it instead of leaving it in place.
func exerciseOffsetsIndexUpgrade(t *testing.T, db *gorm.DB) {
	t.Helper()
	const indexName = "ux_ir_offsets_program_value_file_range"
	require.NoError(t, db.Exec("DROP INDEX IF EXISTS "+QuoteIdent(db, indexName)).Error)
	require.NoError(t, EnsureIndex(db, IndexSpec{
		Name:    indexName,
		Table:   TableIrOffsets,
		Columns: []string{"program_name", "value_id", "file_hash", "start_offset", "end_offset", "variable_name"},
		Unique:  true,
	}))

	ddl, ok := namedIndexDefinition(db, TableIrOffsets, indexName)
	require.True(t, ok)
	require.NotContains(t, strings.ToUpper(ddl), "COALESCE")

	ensureUniqueIrOffsetsIndex(db)
	ddl, ok = namedIndexDefinition(db, TableIrOffsets, indexName)
	require.True(t, ok)
	require.Contains(t, strings.ToUpper(ddl), "COALESCE")
}

// exerciseRedundantIndexPrune installs one index the maintenance pass considers
// redundant and verifies it is actually dropped, not only skipped by HasIndex.
func exerciseRedundantIndexPrune(t *testing.T, db *gorm.DB) {
	t.Helper()
	const indexName = "idx_ir_codes_deleted_at"
	require.NoError(t, EnsureIndex(db, IndexSpec{
		Name:    indexName,
		Table:   TableIrCodes,
		Columns: []string{"deleted_at"},
	}))
	require.True(t, hasNamedIndex(db, TableIrCodes, indexName))

	patchIrCodeIndex(db)
	require.False(t, hasNamedIndex(db, TableIrCodes, indexName))
}

// exerciseAuditNativeReads covers the audit native queries that bypass GORM.
func exerciseAuditNativeReads(t *testing.T, db *gorm.DB, program string) {
	t.Helper()
	nodeID := NewULID()
	targetID := NewULID()
	require.NoError(t, db.Create(&AuditNode{
		AuditNodeStatus: AuditNodeStatus{ProgramName: program},
		NodeID:          nodeID,
	}).Error)
	require.NoError(t, db.Create(&AuditNode{
		AuditNodeStatus: AuditNodeStatus{ProgramName: program},
		NodeID:          targetID,
	}).Error)
	require.NoError(t, db.Create(&AuditEdge{
		ProgramName:   program,
		FromNode:      nodeID,
		ToNode:        targetID,
		EdgeType:      EdgeType_Predecessor,
		AnalysisStep:  3,
		AnalysisLabel: "call",
	}).Error)

	node, err := nativeGetAuditNodeById(db, nodeID)
	require.NoError(t, err)
	require.NotNil(t, node)
	require.Equal(t, nodeID, node.NodeID)

	exportedNode, err := GetAuditNodeById(db, nodeID)
	require.NoError(t, err)
	require.NotNil(t, exportedNode)
	exportedNodes, err := GetAuditNodesByIds(db, []string{nodeID, targetID})
	require.NoError(t, err)
	require.Len(t, exportedNodes, 2)

	require.Equal(t, []string{targetID}, nativeAuditEdgeTargets(db, nodeID, EdgeType_Predecessor))
	edges := nativePredecessorEdges(db, nodeID)
	require.Len(t, edges, 1)
	require.Equal(t, targetID, edges[0].ToNode)
	require.Equal(t, int64(3), edges[0].AnalysisStep)
}
