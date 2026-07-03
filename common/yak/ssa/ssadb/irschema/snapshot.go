package irschema

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/jinzhu/gorm"
)

// SchemaSnapshot is a normalized structural view of a Postgres schema. Two
// databases with equal Snapshots have functionally identical schemas for
// the purposes of IR DDL governance. It intentionally excludes OID columns,
// physical row sizes, and PG-minor-version-dependent rendering artifacts.
//
// SchemaSnapshot powers two consumers:
//   - drift_test.go (the CI maintenance gate): compares a DB built from the
//     baseline SQL against a DB built from GORM AutoMigrate.
//   - Migrate's adoption logic (baselineMatchesActual): decides whether a
//     pre-governance AutoMigrated DB can be stamped at v1 without DDL.
type SchemaSnapshot struct {
	Tables      []string                    `json:"tables"`
	Columns     []ColumnRow                 `json:"columns"`
	Constraints []ConstraintRow             `json:"constraints"`
	Defaults    []DefaultRow                `json:"defaults"`
	Indexes     []IndexRow                  `json:"indexes"`
	Sequences   []SequenceRow               `json:"sequences"`
	Enums       []EnumRow                   `json:"enums"`
}

// ColumnRow captures one information_schema.columns row, normalized.
type ColumnRow struct {
	Table      string `json:"table"`
	Column     string `json:"column"`
	DataType   string `json:"data_type"`
	UDTName    string `json:"udt_name"`
	CharMaxLen *int64 `json:"char_max_len,omitempty"`
	NumPrec    *int64 `json:"num_precision,omitempty"`
	NumScale   *int64 `json:"num_scale,omitempty"`
	IsNullable string `json:"is_nullable"`
	Default    string `json:"default"`
	Position   int64  `json:"position"`
}

type ConstraintRow struct {
	Name    string `json:"name"`
	Type    string `json:"type"` // p/u/c/f
	Table   string `json:"table"`
	Def     string `json:"def"` // pg_get_constraintdef
}

type DefaultRow struct {
	Table string `json:"table"`
	Num   int64  `json:"attnum"`
	Expr  string `json:"expr"` // pg_get_expr(adbin, adrelid)
}

type IndexRow struct {
	Name string `json:"name"`
	Def  string `json:"def"` // pg_indexes.indexdef
}

type SequenceRow struct {
	Name      string `json:"name"`
	Start     int64  `json:"start"`
	Increment int64  `json:"increment"`
	Min       int64  `json:"min"`
	Max       int64  `json:"max"`
	Cache     int64  `json:"cache"`
	Cycle     bool   `json:"cycle"`
}

type EnumRow struct {
	TypeName string `json:"type_name"`
	Label    string `json:"label"`
	SortOrder int64 `json:"sort_order"`
}

// Snapshot reads a normalized structural view of the IR schema from db. The
// db must have SELECT on information_schema, pg_constraint, pg_attrdef,
// pg_indexes, pg_sequence, pg_enum. Safe under a DML-only role.
func Snapshot(ctx context.Context, db *sql.DB) (*SchemaSnapshot, error) {
	s := &SchemaSnapshot{}

	tableRows, err := db.QueryContext(ctx,
		`SELECT table_name FROM information_schema.tables WHERE table_schema='public' ORDER BY table_name`)
	if err != nil {
		return nil, fmt.Errorf("snapshot: tables: %w", err)
	}
	for tableRows.Next() {
		var t string
		tableRows.Scan(&t)
		s.Tables = append(s.Tables, t)
	}
	tableRows.Close()

	// columns
	cols, err := db.QueryContext(ctx, `
        SELECT table_name, column_name, data_type, udt_name,
               character_maximum_length, numeric_precision, numeric_scale,
               is_nullable, column_default, ordinal_position
        FROM information_schema.columns
        WHERE table_schema='public'
        ORDER BY table_name, ordinal_position`)
	if err != nil {
		return nil, fmt.Errorf("snapshot: columns: %w", err)
	}
	for cols.Next() {
		var c ColumnRow
		var charMax, numPrec, numScale sql.NullInt64
		var def sql.NullString
		if err := cols.Scan(&c.Table, &c.Column, &c.DataType, &c.UDTName,
			&charMax, &numPrec, &numScale,
			&c.IsNullable, &def, &c.Position); err != nil {
			cols.Close()
			return nil, err
		}
		if def.Valid {
			c.Default = def.String
		}
		if charMax.Valid {
			v := charMax.Int64
			c.CharMaxLen = &v
		}
		if numPrec.Valid {
			v := numPrec.Int64
			c.NumPrec = &v
		}
		if numScale.Valid {
			v := numScale.Int64
			c.NumScale = &v
		}
		s.Columns = append(s.Columns, c)
	}
	cols.Close()

	// constraints
	cons, err := db.QueryContext(ctx, `
        SELECT c.conname, c.contype, cl.relname, pg_get_constraintdef(c.oid)
        FROM pg_constraint c
        JOIN pg_class cl ON c.conrelid = cl.oid
        JOIN pg_namespace n ON cl.relnamespace = n.oid
        WHERE n.nspname='public'
        ORDER BY c.conname`)
	if err != nil {
		return nil, fmt.Errorf("snapshot: constraints: %w", err)
	}
	for cons.Next() {
		var (
			r    ConstraintRow
			ct   string
		)
		if err := cons.Scan(&r.Name, &ct, &r.Table, &r.Def); err != nil {
			cons.Close()
			return nil, err
		}
		r.Type = ct
		s.Constraints = append(s.Constraints, r)
	}
	cons.Close()

	// defaults
	defs, err := db.QueryContext(ctx, `
        SELECT cl.relname, a.adnum, pg_get_expr(a.adbin, a.adrelid)
        FROM pg_attrdef a
        JOIN pg_class cl ON a.adrelid = cl.oid
        JOIN pg_namespace n ON cl.relnamespace = n.oid
        WHERE n.nspname='public'
        ORDER BY cl.relname, a.adnum`)
	if err != nil {
		return nil, fmt.Errorf("snapshot: defaults: %w", err)
	}
	for defs.Next() {
		var d DefaultRow
		if err := defs.Scan(&d.Table, &d.Num, &d.Expr); err != nil {
			defs.Close()
			return nil, err
		}
		s.Defaults = append(s.Defaults, d)
	}
	defs.Close()

	// indexes
	idx, err := db.QueryContext(ctx,
		`SELECT indexname, indexdef FROM pg_indexes WHERE schemaname='public' ORDER BY indexname`)
	if err != nil {
		return nil, fmt.Errorf("snapshot: indexes: %w", err)
	}
	for idx.Next() {
		var i IndexRow
		if err := idx.Scan(&i.Name, &i.Def); err != nil {
			idx.Close()
			return nil, err
		}
		s.Indexes = append(s.Indexes, i)
	}
	idx.Close()

	// sequences (pg_sequence view is PG 10+)
	seq, err := db.QueryContext(ctx, `
        SELECT seq_name, start_value, increment_by, min_value, max_value, cache_size, cycle
        FROM (
            SELECT c.relname AS seq_name, s.seqstart AS start_value, s.seqincrement AS increment_by,
                   s.seqmin AS min_value, s.seqmax AS max_value, s.seqcache AS cache_size, s.seqcycle AS cycle
            FROM pg_sequence s
            JOIN pg_class c ON s.seqrelid = c.oid
            JOIN pg_namespace n ON c.relnamespace = n.oid
            WHERE n.nspname='public'
        ) q
        ORDER BY seq_name`)
	if err != nil {
		return nil, fmt.Errorf("snapshot: sequences: %w", err)
	}
	for seq.Next() {
		var (
			r        SequenceRow
			cycleStr string
		)
		if err := seq.Scan(&r.Name, &r.Start, &r.Increment, &r.Min, &r.Max, &r.Cache, &cycleStr); err != nil {
			seq.Close()
			return nil, err
		}
		r.Cycle = strings.EqualFold(cycleStr, "t") || strings.EqualFold(cycleStr, "true") || cycleStr == "1"
		s.Sequences = append(s.Sequences, r)
	}
	seq.Close()

	// enums (only relevant if GORM enum types exist in IR schema)
	en, err := db.QueryContext(ctx, `
        SELECT t.typname, e.enumlabel, e.enumsortorder
        FROM pg_type t
        JOIN pg_enum e ON t.oid = e.enumtypid
        JOIN pg_namespace n ON t.typnamespace = n.oid
        WHERE n.nspname='public'
        ORDER BY t.typname, e.enumsortorder`)
	if err != nil {
		return nil, fmt.Errorf("snapshot: enums: %w", err)
	}
	for en.Next() {
		var e EnumRow
		if err := en.Scan(&e.TypeName, &e.Label, &e.SortOrder); err != nil {
			en.Close()
			return nil, err
		}
		s.Enums = append(s.Enums, e)
	}
	en.Close()

	return s, nil
}

// SnapshotFromGorm wraps Snapshot for callers holding a *gorm.DB.
func SnapshotFromGorm(ctx context.Context, db *gorm.DB) (*SchemaSnapshot, error) {
	return Snapshot(ctx, db.DB())
}

// DiffSnapshots compares two snapshots and returns a human-readable report of
// structural differences. Empty string means they are equal. It sorts entries
// before diffing so order does not matter.
func DiffSnapshots(a, b *SchemaSnapshot) string {
	normalize := func(s *SchemaSnapshot) *SchemaSnapshot {
		// Already queried with ORDER BY, but defend against future callers.
		sort.Slice(s.Columns, func(i, j int) bool {
			return keyColumn(s.Columns[i]) < keyColumn(s.Columns[j])
		})
		sort.Slice(s.Constraints, func(i, j int) bool { return s.Constraints[i].Name < s.Constraints[j].Name })
		sort.Slice(s.Defaults, func(i, j int) bool {
			return s.Defaults[i].Table+fmt.Sprintf(":%d", s.Defaults[i].Num) < s.Defaults[j].Table+fmt.Sprintf(":%d", s.Defaults[j].Num)
		})
		sort.Slice(s.Indexes, func(i, j int) bool { return s.Indexes[i].Name < s.Indexes[j].Name })
		sort.Slice(s.Sequences, func(i, j int) bool { return s.Sequences[i].Name < s.Sequences[j].Name })
		sort.Slice(s.Enums, func(i, j int) bool {
			return s.Enums[i].TypeName+":"+s.Enums[i].Label < s.Enums[j].TypeName+":"+s.Enums[j].Label
		})
		return s
	}
	a = normalize(a)
	b = normalize(b)

	aj, _ := json.Marshal(a)
	bj, _ := json.Marshal(b)
	if string(aj) == string(bj) {
		return ""
	}
	// Build a focused diff so the failure message is actionable.
	var sb strings.Builder
	sb.WriteString("schema snapshot drift detected:\n")
	diffStringSets("tables", toStringSet(a.Tables), toStringSet(b.Tables), &sb)
	diffColumns(a.Columns, b.Columns, &sb)
	diffTypedSet("constraints", constraintKeySet(a.Constraints), constraintKeySet(b.Constraints), &sb)
	diffTypedSet("defaults", defaultKeySet(a.Defaults), defaultKeySet(b.Defaults), &sb)
	diffTypedSet("indexes", indexKeySet(a.Indexes), indexKeySet(b.Indexes), &sb)
	diffTypedSet("sequences", sequenceKeySet(a.Sequences), sequenceKeySet(b.Sequences), &sb)
	diffTypedSet("enums", enumKeySet(a.Enums), enumKeySet(b.Enums), &sb)
	return sb.String()
}

func keyColumn(c ColumnRow) string {
	return fmt.Sprintf("%s.%s", c.Table, c.Column)
}

func toStringSet(xs []string) map[string]struct{} {
	m := make(map[string]struct{}, len(xs))
	for _, x := range xs {
		m[x] = struct{}{}
	}
	return m
}

func diffStringSets(label string, a, b map[string]struct{}, sb *strings.Builder) {
	var onlyA, onlyB []string
	for k := range a {
		if _, ok := b[k]; !ok {
			onlyA = append(onlyA, k)
		}
	}
	for k := range b {
		if _, ok := a[k]; !ok {
			onlyB = append(onlyB, k)
		}
	}
	sort.Strings(onlyA)
	sort.Strings(onlyB)
	if len(onlyA) > 0 {
		fmt.Fprintf(sb, "  %s: only in baseline/A: %v\n", label, onlyA)
	}
	if len(onlyB) > 0 {
		fmt.Fprintf(sb, "  %s: only in actual/B:  %v\n", label, onlyB)
	}
}

func diffTypedSet(label string, a, b map[string]struct{}, sb *strings.Builder) {
	diffStringSets(label, a, b, sb)
}

func diffColumns(a, b []ColumnRow, sb *strings.Builder) {
	am := make(map[string]ColumnRow, len(a))
	bm := make(map[string]ColumnRow, len(b))
	for _, c := range a {
		am[keyColumn(c)] = c
	}
	for _, c := range b {
		bm[keyColumn(c)] = c
	}
	keys := mapKeys(am, bm)
	for _, k := range keys {
		ca, oka := am[k]
		cb, okb := bm[k]
		switch {
		case oka && !okb:
			fmt.Fprintf(sb, "  columns: only in baseline/A: %s %+v\n", k, ca)
		case !oka && okb:
			fmt.Fprintf(sb, "  columns: only in actual/B:  %s %+v\n", k, cb)
		default:
			if !equalColumn(ca, cb) {
				fmt.Fprintf(sb, "  columns: mismatch at %s:\n    A: %+v\n    B: %+v\n", k, ca, cb)
			}
		}
	}
}

func equalColumn(a, b ColumnRow) bool {
	return a.Column == b.Column && a.DataType == b.DataType && a.UDTName == b.UDTName &&
		eqInt64p(a.CharMaxLen, b.CharMaxLen) && eqInt64p(a.NumPrec, b.NumPrec) &&
		eqInt64p(a.NumScale, b.NumScale) && a.IsNullable == b.IsNullable &&
		a.Default == b.Default && a.Position == b.Position
}

func eqInt64p(a, b *int64) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}

func mapKeys(maps ...map[string]ColumnRow) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, m := range maps {
		for k := range m {
			if _, ok := seen[k]; ok {
				continue
			}
			seen[k] = struct{}{}
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}

func constraintKeySet(xs []ConstraintRow) map[string]struct{} {
	m := make(map[string]struct{}, len(xs))
	for _, c := range xs {
		m[fmt.Sprintf("%s|%s|%s|%s", c.Name, c.Type, c.Table, c.Def)] = struct{}{}
	}
	return m
}
func defaultKeySet(xs []DefaultRow) map[string]struct{} {
	m := make(map[string]struct{}, len(xs))
	for _, d := range xs {
		m[fmt.Sprintf("%s|%d|%s", d.Table, d.Num, d.Expr)] = struct{}{}
	}
	return m
}
func indexKeySet(xs []IndexRow) map[string]struct{} {
	m := make(map[string]struct{}, len(xs))
	for _, i := range xs {
		m[fmt.Sprintf("%s|%s", i.Name, i.Def)] = struct{}{}
	}
	return m
}
func sequenceKeySet(xs []SequenceRow) map[string]struct{} {
	m := make(map[string]struct{}, len(xs))
	for _, s := range xs {
		m[fmt.Sprintf("%s|%d|%d|%d|%d|%d|%v", s.Name, s.Start, s.Increment, s.Min, s.Max, s.Cache, s.Cycle)] = struct{}{}
	}
	return m
}
func enumKeySet(xs []EnumRow) map[string]struct{} {
	m := make(map[string]struct{}, len(xs))
	for _, e := range xs {
		m[fmt.Sprintf("%s|%s|%d", e.TypeName, e.Label, e.SortOrder)] = struct{}{}
	}
	return m
}
