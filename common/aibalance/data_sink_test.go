package aibalance

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 关键词: data_sink_test, 落盘 append/recent/stats/容量治理 + save 注入冒烟

func TestDataSink_AppendStatsRecent(t *testing.T) {
	s := newDataSink(t.TempDir())
	s.applyConfig(true, 6<<30, 2<<30, 60)

	for i := 1; i <= 3; i++ {
		ok, err := s.appendRecord(map[string]any{"i": i, "model": "m"})
		require.NoError(t, err)
		assert.True(t, ok)
	}
	records, bytes := s.stats()
	assert.Equal(t, int64(3), records)
	assert.Greater(t, bytes, int64(0))

	recent, err := s.recentRecords(2)
	require.NoError(t, err)
	require.Len(t, recent, 2)
	// newest-first: 最后写入的是 i=3。
	assert.EqualValues(t, 3, recent[0]["i"])
	assert.EqualValues(t, 2, recent[1]["i"])
}

func TestDataSink_DisabledIsNoop(t *testing.T) {
	s := newDataSink(t.TempDir())
	s.applyConfig(false, 6<<30, 2<<30, 60)

	ok, err := s.appendRecord(map[string]any{"x": 1})
	require.NoError(t, err)
	assert.False(t, ok)
	records, _ := s.stats()
	assert.Equal(t, int64(0), records)
}

func TestDataSink_RecentDefaultPayload(t *testing.T) {
	s := newDataSink(t.TempDir())
	s.applyConfig(true, 6<<30, 2<<30, 60)

	// 写入跨数量级以验证尾部回扫拿到最新若干条。
	for i := 0; i < 50; i++ {
		_, err := s.appendRecord(map[string]any{"seq": i})
		require.NoError(t, err)
	}
	recent, err := s.recentRecords(5)
	require.NoError(t, err)
	require.Len(t, recent, 5)
	assert.EqualValues(t, 49, recent[0]["seq"])
	assert.EqualValues(t, 45, recent[4]["seq"])
}

func TestDataSink_EnforceCapacityDeletesOldest(t *testing.T) {
	dir := t.TempDir()
	// 手工铺三天分片: 两旧一今。旧分片较大, 今日分片小且需被保留。
	makeShard := func(name string, lines int) {
		path := filepath.Join(dir, name)
		f, err := os.Create(path)
		require.NoError(t, err)
		defer f.Close()
		for i := 0; i < lines; i++ {
			_, _ = f.WriteString(`{"k":"vvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvv"}` + "\n")
		}
	}
	makeShard("2020-01-01.jsonl", 30) // 最旧
	makeShard("2020-01-02.jsonl", 30)
	today := nowDate()
	makeShard(today+dataSinkFileSuffix, 2) // 今日, 必须保留

	s := newDataSink(dir)
	// 让总量超过 maxBytes, 触发回收。每行约 40 字节, 30 行约 1200 字节。
	s.applyConfig(true, 1500, 600, 60)
	s.initCountFromDisk()

	require.NoError(t, s.enforceCapacity())

	// 最旧分片应被删除, 今日分片必须保留。
	_, err := os.Stat(filepath.Join(dir, "2020-01-01.jsonl"))
	assert.True(t, os.IsNotExist(err), "oldest shard should be removed")
	_, err = os.Stat(filepath.Join(dir, today+dataSinkFileSuffix))
	assert.NoError(t, err, "today shard must be preserved")
}

// TestDataSink_IgnoresNonShardFiles 验证落盘目录里混入的非日期分片 .jsonl 文件
// (以及其它后缀文件) 不会被当作镜像数据读取, 也不计入容量统计。
// 关键词: 镜像数据防干扰, isDailyShardName, 非法 jsonl 忽略
func TestDataSink_IgnoresNonShardFiles(t *testing.T) {
	dir := t.TempDir()
	write := func(name, content string) {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644))
	}
	// 合法当天分片: 一条真实记录。
	write(nowDate()+dataSinkFileSuffix, `{"model":"real","i":1}`+"\n")
	// 各类干扰文件: 都不应被读取/统计。
	write("notes.jsonl", `{"model":"junk-a"}`+"\n")            // 非日期命名
	write("2020-1-2.jsonl", `{"model":"junk-b"}`+"\n")        // 日期格式不规范
	write("2020-01-02.jsonl.tmp", `{"model":"junk-c"}`+"\n")  // 临时文件后缀
	write("export.json", `{"model":"junk-d"}`+"\n")           // 非 jsonl
	write(dataSinkIndexFile, `{"records":1,"bytes":1}`)       // sidecar

	s := newDataSink(dir)
	shards, err := s.listShardsLocked()
	require.NoError(t, err)
	require.Len(t, shards, 1, "only the dated daily shard should be recognized")
	assert.Equal(t, nowDate()+dataSinkFileSuffix, shards[0])

	recent, err := s.recentRecords(10)
	require.NoError(t, err)
	require.Len(t, recent, 1, "interfering files must not appear in recent records")
	assert.Equal(t, "real", recent[0]["model"])

	s.initCountFromDisk()
	records, _ := s.stats()
	assert.Equal(t, int64(1), records, "only the dated shard should be counted")
}

func TestDataSink_InitCountFromDiskRecount(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, nowDate()+dataSinkFileSuffix)
	require.NoError(t, os.WriteFile(path, []byte("{\"a\":1}\n{\"a\":2}\n{\"a\":3}\n"), 0o644))

	s := newDataSink(dir)
	s.initCountFromDisk()
	records, bytes := s.stats()
	assert.Equal(t, int64(3), records)
	assert.Greater(t, bytes, int64(0))
}

// TestDataSink_SaveInjectionSmoke 验证 mirror 脚本里 save() 能落盘到全局 sink。
// 引擎不可用的测试环境会跳过。
// 关键词: save 注入冒烟, executeMirrorScript allowPersist
func TestDataSink_SaveInjectionSmoke(t *testing.T) {
	// 用临时目录的 sink 顶替全局单例, 测试后恢复, 避免写到真实 home。
	dir := t.TempDir()
	tmp := newDataSink(dir)
	tmp.applyConfig(true, 6<<30, 2<<30, 60)

	globalDataSinkMu.Lock()
	orig := globalDataSink
	globalDataSink = tmp
	globalDataSinkMu.Unlock()
	defer func() {
		globalDataSinkMu.Lock()
		globalDataSink = orig
		globalDataSinkMu.Unlock()
	}()

	script := `
func handle(data) {
    save()
}
`
	snap := &MirrorSnapshot{ReqID: "save-smoke", Model: "m-smoke"}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	err, _, outcome := executeMirrorScript(ctx, script, snap, true)
	if err != nil {
		t.Skipf("yak script engine not available in test env: %v", err)
	}
	// save() 调用反馈应被正确记录。
	assert.Equal(t, 1, outcome.Calls)
	assert.Equal(t, 1, outcome.Persisted)
	assert.Greater(t, outcome.Bytes, int64(0))
	assert.True(t, outcome.Enabled)
	assert.NotEmpty(t, outcome.Preview)

	recent, rerr := tmp.recentRecords(1)
	require.NoError(t, rerr)
	require.Len(t, recent, 1)
	assert.Equal(t, "m-smoke", recent[0]["model"])
}

// TestExecuteMirrorScript_SaveDryRunNoPersist 验证试运行 (allowPersist=false):
// save() 被记录但不真正落盘。
// 关键词: save 试运行 dry-run, 不落盘只记录
func TestExecuteMirrorScript_SaveDryRunNoPersist(t *testing.T) {
	dir := t.TempDir()
	tmp := newDataSink(dir)
	tmp.applyConfig(true, 6<<30, 2<<30, 60)

	globalDataSinkMu.Lock()
	orig := globalDataSink
	globalDataSink = tmp
	globalDataSinkMu.Unlock()
	defer func() {
		globalDataSinkMu.Lock()
		globalDataSink = orig
		globalDataSinkMu.Unlock()
	}()

	script := `
func handle(data) {
    save()
}
`
	snap := &MirrorSnapshot{ReqID: "dry-run", Model: "m-dry"}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	err, _, outcome := executeMirrorScript(ctx, script, snap, false)
	if err != nil {
		t.Skipf("yak script engine not available in test env: %v", err)
	}
	assert.Equal(t, 1, outcome.Calls)
	assert.Equal(t, 0, outcome.Persisted, "dry-run must not persist")

	recent, rerr := tmp.recentRecords(1)
	require.NoError(t, rerr)
	assert.Empty(t, recent, "dry-run must not write any record")
}
