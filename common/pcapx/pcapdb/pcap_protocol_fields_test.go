package pcapdb

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
)

func TestPCAPDBProtocolFieldTypesAndIndexes(t *testing.T) {
	fields := []string{
		`{"v":true}`, `{"v":false}`, `{"v":1}`, `{"v":0}`, `{"v":1.0}`,
		`{"v":"1"}`, `{"v":null}`, `{}`, `{"v":{"x":"obj"}}`, `{"v":["needle"]}`,
		`{"v":"{\"x\":\"obj\"}"}`,
		`{"headers":{"x.y":"nested"},"records":[{"key":"a"},{"key":"b"}]}`,
		`{"owner's name":"x'); DROP TABLE protocol_messages; --"}`,
		`{"v":"1","tag":"special"}`, `{"v":9223372036854775807,"big":9007199254740993}`,
	}
	m := testManager(t)
	input := writeCapture(t, classicCapture(t, len(fields)))
	db, err := m.GetOrCreate(input, WithProtocols(true))
	require.NoError(t, err)
	meta, err := db.Metadata()
	require.NoError(t, err)
	writer, err := openIndex(meta.DatabasePath, true, false)
	require.NoError(t, err)
	for i, value := range fields {
		require.NoError(t, writer.Model(&PCAPProtocolMessage{}).Where("id = ?", i+1).
			UpdateColumn("fields", gorm.Expr("jsonb(?)", value)).Error)
	}
	require.NoError(t, writer.Close())
	check := func(want []uint, options ...QueryOption) {
		t.Helper()
		rows, err := db.QueryProtocols(options...)
		require.NoError(t, err)
		got := make([]uint, 0, len(rows))
		for _, row := range rows {
			got = append(got, row.ID)
		}
		require.Equal(t, want, got)
	}
	typedQueries := func() {
		check([]uint{1}, QueryField("$.v", true))
		check([]uint{2}, QueryField("$.v", false))
		check([]uint{3, 5}, QueryField("$.v", 1))
		check([]uint{4}, QueryField("$.v", 0))
		check([]uint{6, 14}, QueryField("$.v", "1"))
		check([]uint{7}, QueryField("$.v", nil))
		check([]uint{8, 12, 13}, QueryFieldMissing("$.v"))
		check([]uint{1, 2, 3, 4, 5, 6, 7, 9, 10, 11, 14, 15}, QueryFieldExists("$.v"))
		check([]uint{11}, QueryField("$.v", `{"x":"obj"}`))
		check([]uint{14}, QueryField("$.v", "1"), QueryField("$.tag", "special"))
		check([]uint{}, QueryField("$.v", "1"), QueryField("$.tag", "absent"))
		check([]uint{12}, QueryField(`$.headers."x.y"`, "nested"), QueryField("$.records[1].key", "b"))
		check([]uint{12}, QueryField("$.records[#-1].key", "b"))
		check([]uint{13}, QueryField(`$."owner's name"`, "x'); DROP TABLE protocol_messages; --"))
		check([]uint{15}, QueryField("$.v", uint64(math.MaxInt64)))
		check([]uint{15}, QueryField("$.big", json.Number("9007199254740993")))
		check([]uint{14}, QueryField("$.v", "1"), QueryAfter(6), QueryLimit(1))
	}
	typedQueries()
	detail, err := db.ProtocolDetails(15)
	require.NoError(t, err)
	require.Equal(t, json.Number("9007199254740993"), detail.Fields["big"], "field decoding must preserve integers beyond float64 precision")
	var states []State
	again, err := m.GetOrCreate(input, WithProtocols(true), WithFieldIndex("$.v"), WithFieldIndex("$.tag"),
		WithFieldIndex(`$."owner's name"`), WithFieldIndex("$.v"),
		WithProgress(func(progress Progress) { states = append(states, progress.State) }))
	require.NoError(t, err)
	require.Equal(t, db.ID, again.ID)
	require.NotContains(t, states, StateAnalyzing, "adding indexes must not replay or overwrite existing fields")
	typedQueries()
	var indexes int
	require.NoError(t, db.db.Table("sqlite_schema").Where("type = ? AND name IN (?)", "index",
		[]string{protocolFieldIndexName("$.v"), protocolFieldIndexName("$.tag"), protocolFieldIndexName(`$."owner's name"`)}).Count(&indexes).Error)
	require.Equal(t, 3, indexes)
	var profileIndexes int
	require.NoError(t, m.profile.Table("sqlite_schema").Where("substr(name,1,?) = ?", len(protocolFieldIndexPrefix), protocolFieldIndexPrefix).Count(&profileIndexes).Error)
	require.Zero(t, profileIndexes, "field indexes must only exist in the traffic sub-DB")
	assertProtocolFieldQueryPlan(t, db.db, "$.v", "1")
	_, err = m.GetOrCreate(input, WithProtocols(true), WithFieldIndex("$.v"))
	require.NoError(t, err)
	require.NoError(t, db.Close())
	reopened, err := m.Open(context.Background(), db.ID)
	require.NoError(t, err)
	rows, err := reopened.QueryProtocols(QueryField("$.v", nil))
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.EqualValues(t, 7, rows[0].ID)
}

func assertProtocolFieldQueryPlan(t testing.TB, db *gorm.DB, path string, value any) {
	t.Helper()
	// The same predicate builder is used by the public query and by this plan
	// check, including the literal path and the partial-index condition.
	predicate := protocolFieldPredicate{path: path, value: value}
	query := predicate.apply(db.Model(&PCAPProtocolMessage{})).Where("id > ?", 0).Order("id").Limit(10)
	rows, err := db.Raw("EXPLAIN QUERY PLAN ?", query.QueryExpr()).Rows()
	require.NoError(t, err)
	defer rows.Close()
	var plans []string
	for rows.Next() {
		var id, parent, unused int
		var detail string
		require.NoError(t, rows.Scan(&id, &parent, &unused, &detail))
		plans = append(plans, detail)
	}
	require.NoError(t, rows.Err())
	plan := strings.Join(plans, "\n")
	require.Contains(t, plan, "SEARCH protocol_messages USING INDEX "+protocolFieldIndexName(path))
	require.NotContains(t, plan, "USE TEMP B-TREE")
	t.Log(plan)
}

func TestPCAPDBProtocolFieldValidationAndAtomicIndexLimit(t *testing.T) {
	for _, path := range []string{"", "v", "$..v", "$.", "$[", "$[1", "$[-1]", "$[#]", "$[#-0]", "$[*]", `$."unterminated`, "$.x\x00", "$" + strings.Repeat(".x", 65)} {
		_, err := parseQuery([]QueryOption{QueryField(path, 1)}, true)
		require.Error(t, err, path)
	}
	for _, value := range []any{math.NaN(), math.Inf(1), uint64(math.MaxInt64) + 1, json.Number("9223372036854775808"), []string{"a"}, map[string]any{"a": 1}} {
		_, err := parseQuery([]QueryOption{QueryField("$.v", value)}, true)
		require.Error(t, err)
	}
	options := make([]QueryOption, maxProtocolFieldPredicates+1)
	for i := range options {
		options[i] = QueryField("$.v", i)
	}
	_, err := parseQuery(options, true)
	require.Error(t, err)
	_, err = parseQuery([]QueryOption{QueryFieldExists("$.v")}, false)
	require.Error(t, err)
	m := testManager(t)
	input := writeCapture(t, classicCapture(t, 1))
	_, err = m.GetOrCreate(input, WithFieldIndex("$.v"))
	require.ErrorContains(t, err, "require withProtocols(true)")
	db, err := m.GetOrCreate(input, WithProtocols(true), WithFieldIndex("$.v"))
	require.NoError(t, err)
	meta, err := db.Metadata()
	require.NoError(t, err)
	writer, err := openIndex(meta.DatabasePath, true, false)
	require.NoError(t, err)
	defer writer.Close()
	paths := make([]string, maxProtocolFieldIndexes-2)
	for i := range paths {
		paths[i] = fmt.Sprintf("$.field%d", i)
	}
	require.NoError(t, ensureProtocolFieldIndexes(context.Background(), writer, paths))
	// One slot remains. The first of two additions must also roll back when
	// the second would exceed the database-wide limit.
	err = ensureProtocolFieldIndexes(context.Background(), writer, []string{"$.last", "$.overflow"})
	require.Error(t, err)
	var count int
	require.NoError(t, writer.Table("sqlite_schema").Where("name = ?", protocolFieldIndexName("$.last")).Count(&count).Error)
	require.Zero(t, count)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.ErrorIs(t, ensureProtocolFieldIndexes(ctx, writer, []string{"$.cancelled"}), context.Canceled)
	rows, err := db.QueryProtocols(QueryProtocol("dns"))
	require.NoError(t, err)
	require.Len(t, rows, 1, "index failure must preserve Ready data")
	// Reader refresh waits for active queries, but import cancellation must
	// interrupt that wait rather than hanging behind an unrelated long scan.
	db.mu.RLock()
	ctx, cancel = context.WithCancel(context.Background())
	refreshed := make(chan error, 1)
	go func() { refreshed <- m.refreshReader(ctx, meta) }()
	cancel()
	select {
	case err := <-refreshed:
		db.mu.RUnlock()
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(time.Second):
		db.mu.RUnlock()
		t.Fatal("reader refresh did not honor cancellation")
	}
}

// This synthetic store isolates field-search cost from capture replay cost.
// Fields stay JSONB and records use the same GORM model and public query path.
func protocolFieldBenchmarkStore(b *testing.B, count int) (*Database, *gorm.DB) {
	b.Helper()
	m := testManager(b)
	db, err := m.GetOrCreate(writeCapture(b, classicCapture(b, 1)))
	require.NoError(b, err)
	meta, err := db.Metadata()
	require.NoError(b, err)
	writer, err := openIndex(meta.DatabasePath, true, false)
	require.NoError(b, err)
	b.Cleanup(func() { require.NoError(b, writer.Close()) })
	tx := writer.Begin()
	require.NoError(b, tx.Error)
	defer tx.Rollback()
	for start := 1; start <= count; start += 1000 {
		messages := make([]PCAPProtocolMessage, 0, 1000)
		for id := start; id <= count && id < start+1000; id++ {
			key := "common"
			if id == count {
				key = "needle"
			}
			fields, err := json.Marshal(map[string]any{
				"Method": key, "Status": id % 10, "Payload": strings.Repeat("payload-", 64),
				"Nested": map[string]any{"ID": id, "Enabled": true},
			})
			require.NoError(b, err)
			messages = append(messages, PCAPProtocolMessage{
				Data:  []byte{},
				Model: gorm.Model{ID: uint(id)}, EventID: int64(id), Protocol: "test-rpc",
				FieldsJSON: fields, SessionJSON: []byte("null"), SourceBytesJSON: []byte("null"),
			})
		}
		require.NoError(b, tx.CreateInBatches(&messages).Error)
		require.NoError(b, tx.Model(&PCAPProtocolMessage{}).Where("id >= ? AND id < ?", start, start+1000).
			UpdateColumn("fields", gorm.Expr("jsonb(CAST(fields AS TEXT))")).Error)
	}
	meta.ProtocolsIndexed, meta.ProtocolCount = true, int64(count)
	require.NoError(b, writeManifest(context.Background(), tx, meta))
	require.NoError(b, tx.Commit().Error)
	return db, writer
}

func BenchmarkPCAPDBProtocolFieldSearch(b *testing.B) {
	const count = 50000
	db, writer := protocolFieldBenchmarkStore(b, count)
	bench := func(name, value string, wanted int, build time.Duration, indexBytes int64) {
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				rows, err := db.QueryProtocols(QueryProtocol("test-rpc"), QueryField("$.Method", value), QueryLimit(10))
				require.NoError(b, err)
				require.Len(b, rows, wanted)
			}
			b.ReportMetric(count, "messages")
			if build != 0 {
				b.ReportMetric(float64(build.Microseconds())/1000, "build_ms")
				b.ReportMetric(float64(indexBytes)/1024, "index_KiB")
			}
		})
	}
	bench("unindexed_hit", "needle", 1, 0, 0)
	bench("unindexed_miss", "absent", 0, 0, 0)
	var before, after, pageSize int64
	require.NoError(b, writer.Raw("PRAGMA page_count").Row().Scan(&before))
	require.NoError(b, writer.Raw("PRAGMA page_size").Row().Scan(&pageSize))
	started := time.Now()
	require.NoError(b, ensureProtocolFieldIndexes(context.Background(), writer, []string{"$.Method"}))
	build := time.Since(started)
	require.NoError(b, writer.Raw("PRAGMA page_count").Row().Scan(&after))
	meta, err := db.Metadata()
	require.NoError(b, err)
	require.NoError(b, db.manager.refreshReader(context.Background(), meta))
	assertProtocolFieldQueryPlan(b, db.db, "$.Method", "needle")
	bench("indexed_hit", "needle", 1, build, (after-before)*pageSize)
	bench("indexed_miss", "absent", 0, build, (after-before)*pageSize)
}
