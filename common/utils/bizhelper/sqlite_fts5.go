package bizhelper

import (
	"context"
	"fmt"
	"strings"

	"github.com/jinzhu/gorm"
)

// SQLiteFTS5Config describes how to build and maintain a FTS5 virtual table that indexes an existing table.
//
// Notes:
// - This helper does NOT "enable" FTS; it assumes your SQLite build already includes FTS5.
// - RowIDColumn should be an INTEGER primary key (or at least stable integer) to map into FTS rowid.
type SQLiteFTS5Config struct {
	// BaseModel is optional; if BaseTable is empty and BaseModel is set, BaseTable is inferred via gorm scope.
	BaseModel any

	// BaseTable is the original table name to index.
	BaseTable string

	// FTSTable is the FTS5 virtual table name to create.
	FTSTable string

	// RowIDColumn is the integer primary key column in BaseTable used to map to FTS rowid (default: "id").
	RowIDColumn string

	// Columns are the BaseTable columns to index in the FTS5 table (e.g. []string{"title","body"}).
	Columns []string

	// Tokenize is optional FTS5 tokenize spec (default: "unicode61").
	// Example: "unicode61 remove_diacritics 2"
	Tokenize string

	// ContentTable enables "external content" mode (FTS5 content='...').
	// If empty, creates a standalone FTS5 table and migration uses INSERT..SELECT.
	// If non-empty, migration uses the FTS5 'rebuild' command.
	ContentTable string

	// ContentRowID sets content_rowid for external content (default: RowIDColumn).
	ContentRowID string
}

type SQLiteFTS5Option func(*SQLiteFTS5Config)

func WithSQLiteFTS5BaseModel(m any) SQLiteFTS5Option {
	return func(c *SQLiteFTS5Config) {
		c.BaseModel = m
	}
}

func WithSQLiteFTS5BaseTable(table string) SQLiteFTS5Option {
	return func(c *SQLiteFTS5Config) {
		c.BaseTable = table
	}
}

func WithSQLiteFTS5FTSTable(table string) SQLiteFTS5Option {
	return func(c *SQLiteFTS5Config) {
		c.FTSTable = table
	}
}

func WithSQLiteFTS5RowIDColumn(col string) SQLiteFTS5Option {
	return func(c *SQLiteFTS5Config) {
		c.RowIDColumn = col
	}
}

func WithSQLiteFTS5Columns(cols ...string) SQLiteFTS5Option {
	return func(c *SQLiteFTS5Config) {
		c.Columns = append([]string(nil), cols...)
	}
}

func WithSQLiteFTS5Tokenize(tokenize string) SQLiteFTS5Option {
	return func(c *SQLiteFTS5Config) {
		c.Tokenize = tokenize
	}
}

func WithSQLiteFTS5ExternalContent(contentTable, contentRowID string) SQLiteFTS5Option {
	return func(c *SQLiteFTS5Config) {
		c.ContentTable = contentTable
		c.ContentRowID = contentRowID
	}
}

func (c *SQLiteFTS5Config) apply(opts ...SQLiteFTS5Option) {
	for _, opt := range opts {
		if opt == nil {
			continue
		}
		opt(c)
	}
}

func (c *SQLiteFTS5Config) normalize(db *gorm.DB) error {
	if c == nil {
		return fmt.Errorf("nil fts5 config")
	}

	if c.BaseTable == "" && c.BaseModel != nil {
		c.BaseTable = db.NewScope(c.BaseModel).TableName()
	}

	if c.RowIDColumn == "" {
		c.RowIDColumn = "id"
	}
	if c.Tokenize == "" {
		c.Tokenize = "unicode61"
	}
	if c.ContentTable != "" && c.ContentRowID == "" {
		c.ContentRowID = c.RowIDColumn
	}

	if c.BaseTable == "" {
		return fmt.Errorf("BaseTable is empty")
	}
	if c.FTSTable == "" {
		return fmt.Errorf("FTSTable is empty")
	}
	if c.RowIDColumn == "" {
		return fmt.Errorf("RowIDColumn is empty")
	}
	if len(c.Columns) == 0 {
		return fmt.Errorf("Columns is empty")
	}
	for _, col := range c.Columns {
		if strings.TrimSpace(col) == "" {
			return fmt.Errorf("Columns contains empty column")
		}
	}
	return nil
}

func sqliteFTS5ResolveConfig(db *gorm.DB, base *SQLiteFTS5Config, opts ...SQLiteFTS5Option) (*SQLiteFTS5Config, error) {
	if db == nil {
		return nil, fmt.Errorf("nil db")
	}
	var cfg SQLiteFTS5Config
	if base != nil {
		cfg = *base
	}
	cfg.apply(opts...)
	if err := cfg.normalize(db); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func sqliteQuoteIdent(name string) string {
	// Double-quote identifiers and escape internal quotes by doubling.
	// This keeps DDL/DML safe even when names come from configuration.
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

func sqliteQuoteStringLiteral(s string) string {
	return `'` + strings.ReplaceAll(s, `'`, `''`) + `'`
}

func sqliteFTS5CreateVirtualTableResolved(db *gorm.DB, cfg *SQLiteFTS5Config) error {
	colDefs := make([]string, 0, len(cfg.Columns))
	for _, col := range cfg.Columns {
		colDefs = append(colDefs, sqliteQuoteIdent(col))
	}

	tableOpts := make([]string, 0, 4)
	if cfg.Tokenize != "" {
		tableOpts = append(tableOpts, "tokenize="+sqliteQuoteStringLiteral(cfg.Tokenize))
	}
	// Default to contentless mode for standalone FTS tables.
	// This makes maintenance commands ('delete', 'delete-all') and trigger-based syncing work reliably
	// across SQLite builds, and avoids storing duplicated content in the FTS table.
	if cfg.ContentTable == "" {
		tableOpts = append(tableOpts, "content="+sqliteQuoteStringLiteral(""))
	}
	if cfg.ContentTable != "" {
		tableOpts = append(tableOpts, "content="+sqliteQuoteStringLiteral(cfg.ContentTable))
		tableOpts = append(tableOpts, "content_rowid="+sqliteQuoteStringLiteral(cfg.ContentRowID))
	}

	args := strings.Join(colDefs, ", ")
	if len(tableOpts) > 0 {
		args = args + ", " + strings.Join(tableOpts, ", ")
	}

	sql := fmt.Sprintf("CREATE VIRTUAL TABLE IF NOT EXISTS %s USING fts5(%s);", sqliteQuoteIdent(cfg.FTSTable), args)
	return db.Exec(sql).Error
}

func sqliteFTS5MigrateDataResolved(db *gorm.DB, cfg *SQLiteFTS5Config) error {
	ftsTable := sqliteQuoteIdent(cfg.FTSTable)
	ftsCmdCol := sqliteQuoteIdent(cfg.FTSTable)

	if cfg.ContentTable != "" {
		// External content mode: rebuild from content table.
		rebuildSQL := fmt.Sprintf("INSERT INTO %s(%s) VALUES('rebuild');", ftsTable, ftsCmdCol)
		return db.Exec(rebuildSQL).Error
	}

	// Standalone FTS table: clear then bulk insert from BaseTable.
	// Prefer the fast maintenance command, but fall back for SQLite builds that reject it.
	deleteAllSQL := fmt.Sprintf("INSERT INTO %s(%s) VALUES('delete-all');", ftsTable, ftsCmdCol)
	if err := db.Exec(deleteAllSQL).Error; err != nil {
		// Some SQLite/FTS5 builds only allow 'delete-all' for contentless or external-content tables.
		// Fallback to a plain DELETE, which is broadly supported.
		if err2 := db.Exec(fmt.Sprintf("DELETE FROM %s;", ftsTable)).Error; err2 != nil {
			return err
		}
	}

	selectCols := make([]string, 0, len(cfg.Columns)+1)
	selectCols = append(selectCols, "b."+sqliteQuoteIdent(cfg.RowIDColumn))
	for _, col := range cfg.Columns {
		selectCols = append(selectCols, "b."+sqliteQuoteIdent(col))
	}

	insertCols := make([]string, 0, len(cfg.Columns)+1)
	insertCols = append(insertCols, "rowid")
	for _, col := range cfg.Columns {
		insertCols = append(insertCols, sqliteQuoteIdent(col))
	}

	insertSQL := fmt.Sprintf(
		"INSERT INTO %s(%s) SELECT %s FROM %s AS b;",
		ftsTable,
		strings.Join(insertCols, ", "),
		strings.Join(selectCols, ", "),
		sqliteQuoteIdent(cfg.BaseTable),
	)
	return db.Exec(insertSQL).Error
}

func sqliteFTS5BindTriggersResolved(db *gorm.DB, cfg *SQLiteFTS5Config) error {
	base := sqliteQuoteIdent(cfg.BaseTable)
	fts := sqliteQuoteIdent(cfg.FTSTable)
	ftsCmdCol := sqliteQuoteIdent(cfg.FTSTable)
	pk := sqliteQuoteIdent(cfg.RowIDColumn)

	triggerPrefix := fmt.Sprintf("%s_%s_fts5", cfg.BaseTable, cfg.FTSTable)
	ai := sqliteQuoteIdent(triggerPrefix + "_ai")
	au := sqliteQuoteIdent(triggerPrefix + "_au")
	ad := sqliteQuoteIdent(triggerPrefix + "_ad")

	for _, s := range []string{
		fmt.Sprintf("DROP TRIGGER IF EXISTS %s;", ai),
		fmt.Sprintf("DROP TRIGGER IF EXISTS %s;", au),
		fmt.Sprintf("DROP TRIGGER IF EXISTS %s;", ad),
	} {
		if err := db.Exec(s).Error; err != nil {
			return err
		}
	}

	colsCSV := make([]string, 0, len(cfg.Columns))
	newVals := make([]string, 0, len(cfg.Columns))
	oldVals := make([]string, 0, len(cfg.Columns))
	for _, col := range cfg.Columns {
		qc := sqliteQuoteIdent(col)
		colsCSV = append(colsCSV, qc)
		newVals = append(newVals, "new."+qc)
		oldVals = append(oldVals, "old."+qc)
	}

	insertCols := "rowid"
	if len(colsCSV) > 0 {
		insertCols += ", " + strings.Join(colsCSV, ", ")
	}

	newInsertVals := "new." + sqliteQuoteIdent(cfg.RowIDColumn)
	if len(newVals) > 0 {
		newInsertVals += ", " + strings.Join(newVals, ", ")
	}

	deleteColsSuffix := ""
	if len(colsCSV) > 0 {
		deleteColsSuffix = ", " + strings.Join(colsCSV, ", ")
	}

	aiSQL := fmt.Sprintf(`CREATE TRIGGER %s AFTER INSERT ON %s BEGIN
  INSERT INTO %s(%s) VALUES (%s);
END;`, ai, base, fts, insertCols, newInsertVals)

	// For FTS5 'delete', it is safer to include the old column values when available.
	oldDeleteValsSuffix := ""
	if len(oldVals) > 0 {
		oldDeleteValsSuffix = ", " + strings.Join(oldVals, ", ")
	}
	adSQL := fmt.Sprintf(`CREATE TRIGGER %s AFTER DELETE ON %s BEGIN
  INSERT INTO %s(%s, rowid%s) VALUES ('delete', old.%s%s);
END;`, ad, base, fts, ftsCmdCol, deleteColsSuffix, pk, oldDeleteValsSuffix)

	auSQL := fmt.Sprintf(`CREATE TRIGGER %s AFTER UPDATE ON %s BEGIN
  INSERT INTO %s(%s, rowid%s) VALUES ('delete', old.%s%s);
  INSERT INTO %s(%s) VALUES (%s);
END;`,
		au, base,
		fts, ftsCmdCol, deleteColsSuffix, pk, oldDeleteValsSuffix,
		fts, insertCols, newInsertVals,
	)

	for _, s := range []string{aiSQL, adSQL, auSQL} {
		if err := db.Exec(s).Error; err != nil {
			return err
		}
	}
	return nil
}

func sqliteFTS5DropTriggersResolved(db *gorm.DB, cfg *SQLiteFTS5Config) error {
	triggerPrefix := fmt.Sprintf("%s_%s_fts5", cfg.BaseTable, cfg.FTSTable)
	ai := sqliteQuoteIdent(triggerPrefix + "_ai")
	au := sqliteQuoteIdent(triggerPrefix + "_au")
	ad := sqliteQuoteIdent(triggerPrefix + "_ad")

	for _, s := range []string{
		fmt.Sprintf("DROP TRIGGER IF EXISTS %s;", ai),
		fmt.Sprintf("DROP TRIGGER IF EXISTS %s;", au),
		fmt.Sprintf("DROP TRIGGER IF EXISTS %s;", ad),
	} {
		if err := db.Exec(s).Error; err != nil {
			return err
		}
	}
	return nil
}

// SQLiteFTS5CreateVirtualTable creates the FTS5 virtual table if it doesn't exist.
// baseCfg is treated as a template; options are applied on a copy, so baseCfg is not mutated.
func SQLiteFTS5CreateVirtualTable(db *gorm.DB, baseCfg *SQLiteFTS5Config, opts ...SQLiteFTS5Option) error {
	cfg, err := sqliteFTS5ResolveConfig(db, baseCfg, opts...)
	if err != nil {
		return err
	}
	return sqliteFTS5CreateVirtualTableResolved(db, cfg)
}

// SQLiteFTS5MigrateData builds (or rebuilds) the FTS index from BaseTable.
// baseCfg is treated as a template; options are applied on a copy, so baseCfg is not mutated.
func SQLiteFTS5MigrateData(db *gorm.DB, baseCfg *SQLiteFTS5Config, opts ...SQLiteFTS5Option) error {
	cfg, err := sqliteFTS5ResolveConfig(db, baseCfg, opts...)
	if err != nil {
		return err
	}
	if err := sqliteFTS5CreateVirtualTableResolved(db, cfg); err != nil {
		return err
	}
	return sqliteFTS5MigrateDataResolved(db, cfg)
}

// SQLiteFTS5BindTriggers creates (or recreates) triggers to keep FTSTable in sync with BaseTable.
// baseCfg is treated as a template; options are applied on a copy, so baseCfg is not mutated.
func SQLiteFTS5BindTriggers(db *gorm.DB, baseCfg *SQLiteFTS5Config, opts ...SQLiteFTS5Option) error {
	cfg, err := sqliteFTS5ResolveConfig(db, baseCfg, opts...)
	if err != nil {
		return err
	}
	if err := sqliteFTS5CreateVirtualTableResolved(db, cfg); err != nil {
		return err
	}
	return sqliteFTS5BindTriggersResolved(db, cfg)
}

// SQLiteFTS5Drop drops triggers and the FTS5 virtual table (best-effort).
// This is useful when the base table is dropped and the orphan FTS artifacts should be cleaned up.
// baseCfg is treated as a template; options are applied on a copy, so baseCfg is not mutated.
func SQLiteFTS5Drop(db *gorm.DB, baseCfg *SQLiteFTS5Config, opts ...SQLiteFTS5Option) error {
	if db == nil {
		return fmt.Errorf("nil db")
	}
	cfg, err := sqliteFTS5ResolveConfig(db, baseCfg, opts...)
	if err != nil {
		return err
	}

	// Triggers may already be removed automatically if BaseTable was dropped; keep it idempotent.
	if err := sqliteFTS5DropTriggersResolved(db, cfg); err != nil {
		return err
	}

	// Dropping the virtual table is enough to remove the FTS index data.
	return db.Exec(fmt.Sprintf("DROP TABLE IF EXISTS %s;", sqliteQuoteIdent(cfg.FTSTable))).Error
}

// SQLiteFTS5Setup is a convenience wrapper: create table, migrate existing data, bind triggers (in a transaction).
// baseCfg is treated as a template; options are applied on a copy, so baseCfg is not mutated.
func SQLiteFTS5Setup(db *gorm.DB, baseCfg *SQLiteFTS5Config, opts ...SQLiteFTS5Option) error {
	if db == nil {
		return fmt.Errorf("nil db")
	}
	tx := db.Begin()
	if tx.Error != nil {
		return tx.Error
	}

	cfg, err := sqliteFTS5ResolveConfig(tx, baseCfg, opts...)
	if err != nil {
		tx.Rollback()
		return err
	}

	if err := sqliteFTS5CreateVirtualTableResolved(tx, cfg); err != nil {
		tx.Rollback()
		return err
	}
	if err := sqliteFTS5MigrateDataResolved(tx, cfg); err != nil {
		tx.Rollback()
		return err
	}
	if err := sqliteFTS5BindTriggersResolved(tx, cfg); err != nil {
		tx.Rollback()
		return err
	}

	return tx.Commit().Error
}

// SQLiteFTS5BM25MatchInto performs a BM25 ranked FTS5 MATCH search and scans results into dest.
// dest should be a pointer to a slice of the base table struct (e.g. *[]MyModel) or a pointer to a struct.
// baseCfg is treated as a template; options are applied on a copy, so baseCfg is not mutated.
func SQLiteFTS5BM25MatchInto(db *gorm.DB, baseCfg *SQLiteFTS5Config, match string, dest any, limit, offset int, opts ...SQLiteFTS5Option) error {
	if db == nil {
		return fmt.Errorf("nil db")
	}
	if dest == nil {
		return fmt.Errorf("nil dest")
	}
	if strings.TrimSpace(match) == "" {
		// Keep behavior predictable: empty query returns empty result set.
		return nil
	}
	if limit <= 0 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}

	cfg, err := sqliteFTS5ResolveConfig(db, baseCfg, opts...)
	if err != nil {
		return err
	}

	// NOTE: For gorm query builder, avoid passing quoted identifiers (gorm will quote them again).
	// Use plain names here; they are internal (not user-controlled) and come from cfg.
	fts := cfg.FTSTable
	base := cfg.BaseTable
	pk := cfg.RowIDColumn

	// Use gorm builder (instead of Raw) so caller-provided filters on db (Where/Joins/etc.)
	// are preserved and applied to the base table.
	q := db.Table(base).
		Select(base+".*").
		Joins("JOIN "+fts+" ON "+fts+".rowid = "+base+"."+pk).
		Where(fts+" MATCH ?", match).
		Order("bm25(" + fts + ")").
		Limit(limit).
		Offset(offset)
	return q.Scan(dest).Error
}

// SQLiteFTS5BM25Match performs a BM25 ranked FTS5 MATCH search and returns base-table structs.
// baseCfg is treated as a template; options are applied on a copy, so baseCfg is not mutated.
func SQLiteFTS5BM25Match[T any](db *gorm.DB, baseCfg *SQLiteFTS5Config, match string, limit, offset int, opts ...SQLiteFTS5Option) ([]T, error) {
	var out []T
	if err := SQLiteFTS5BM25MatchInto(db, baseCfg, match, &out, limit, offset, opts...); err != nil {
		return nil, err
	}
	return out, nil
}

// SQLiteFTS5BM25MatchYield yields results of a BM25 ranked FTS5 MATCH search in pages.
// It preserves caller-provided filters on db (Where/Joins/etc.) and applies them on the base table.
//
// YieldModelOpts are supported:
// - WithYieldModel_PageSize(n)
// - WithYieldModel_Limit(n)
// - WithYieldModel_CountCallback(func(total int){...}) (best-effort)
func SQLiteFTS5BM25MatchYield[T any](ctx context.Context, db *gorm.DB, baseCfg *SQLiteFTS5Config, match string, yieldOpts ...YieldModelOpts) chan T {
	outC := make(chan T)
	go func() {
		defer close(outC)

		if db == nil {
			return
		}
		match = strings.TrimSpace(match)
		if match == "" {
			return
		}

		cfg := NewYieldModelConfig()
		for _, opt := range yieldOpts {
			if opt == nil {
				continue
			}
			opt(cfg)
		}
		if cfg.Size <= 0 {
			cfg.Size = defaultYieldSize
		}

		if cfg.CountCallback != nil {
			if resolved, err := sqliteFTS5ResolveConfig(db, baseCfg); err == nil {
				var count int
				base := resolved.BaseTable
				fts := resolved.FTSTable
				pk := resolved.RowIDColumn
				// Best-effort count; ignore errors.
				_ = db.Table(base).
					Joins("JOIN "+fts+" ON "+fts+".rowid = "+base+"."+pk).
					Where(fts+" MATCH ?", match).
					Count(&count).Error
				cfg.CountCallback(count)
			}
		}

		total := 0
		for offset := 0; ; offset += cfg.Size {
			items, err := SQLiteFTS5BM25Match[T](db, baseCfg, match, cfg.Size, offset)
			if err != nil {
				return
			}
			if len(items) == 0 {
				return
			}
			for _, item := range items {
				select {
				case <-ctx.Done():
					return
				case outC <- item:
					total++
					if cfg.Limit > 0 && total >= cfg.Limit {
						return
					}
				}
			}
		}
	}()
	return outC
}
