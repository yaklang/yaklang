package irschema

import (
	"context"
	"database/sql"
	"strings"
)

// snapshotTx builds a SchemaSnapshot from a *sql.Tx, used by Migrate's adoption
// logic which runs inside the bootstrap transaction. It mirrors SchemaSnapshot
// (which takes *sql.DB) but executes against an open tx.
func snapshotTx(ctx context.Context, tx *sql.Tx) (*SchemaSnapshot, error) {
	s := &SchemaSnapshot{}

	tableRows, err := tx.QueryContext(ctx,
		`SELECT table_name FROM information_schema.tables WHERE table_schema='public' ORDER BY table_name`)
	if err != nil {
		return nil, err
	}
	for tableRows.Next() {
		var t string
		tableRows.Scan(&t)
		s.Tables = append(s.Tables, t)
	}
	tableRows.Close()

	cols, err := tx.QueryContext(ctx, `
        SELECT table_name, column_name, data_type, udt_name,
               character_maximum_length, numeric_precision, numeric_scale,
               is_nullable, column_default, ordinal_position
        FROM information_schema.columns
        WHERE table_schema='public'
        ORDER BY table_name, ordinal_position`)
	if err != nil {
		return nil, err
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

	cons, err := tx.QueryContext(ctx, `
        SELECT c.conname, c.contype, cl.relname, pg_get_constraintdef(c.oid)
        FROM pg_constraint c
        JOIN pg_class cl ON c.conrelid = cl.oid
        JOIN pg_namespace n ON cl.relnamespace = n.oid
        WHERE n.nspname='public'
        ORDER BY c.conname`)
	if err != nil {
		return nil, err
	}
	for cons.Next() {
		var r ConstraintRow
		var ct string
		if err := cons.Scan(&r.Name, &ct, &r.Table, &r.Def); err != nil {
			cons.Close()
			return nil, err
		}
		r.Type = ct
		s.Constraints = append(s.Constraints, r)
	}
	cons.Close()

	defs, err := tx.QueryContext(ctx, `
        SELECT cl.relname, a.adnum, pg_get_expr(a.adbin, a.adrelid)
        FROM pg_attrdef a
        JOIN pg_class cl ON a.adrelid = cl.oid
        JOIN pg_namespace n ON cl.relnamespace = n.oid
        WHERE n.nspname='public'
        ORDER BY cl.relname, a.adnum`)
	if err != nil {
		return nil, err
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

	idx, err := tx.QueryContext(ctx,
		`SELECT indexname, indexdef FROM pg_indexes WHERE schemaname='public' ORDER BY indexname`)
	if err != nil {
		return nil, err
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

	seq, err := tx.QueryContext(ctx, `
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
		return nil, err
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

	en, err := tx.QueryContext(ctx, `
        SELECT t.typname, e.enumlabel, e.enumsortorder
        FROM pg_type t
        JOIN pg_enum e ON t.oid = e.enumtypid
        JOIN pg_namespace n ON t.typnamespace = n.oid
        WHERE n.nspname='public'
        ORDER BY t.typname, e.enumsortorder`)
	if err != nil {
		return nil, err
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
