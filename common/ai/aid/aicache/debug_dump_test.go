package aicache

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 关键词: aicache, debug dump, render 格式
func TestRenderDebugDump_FullFormat(t *testing.T) {
	gc := newGlobalCache(8)
	prompt := buildFourSectionPrompt("nx", "user q", "tools-x", "static-x", "tl-x", "mem-x")
	split := Split(prompt)

	// 提前注册一次让 ChunkInfo 有 firstSeen / hitCount
	_ = gc.Record(split, "qwen-plus")
	rep := gc.Record(split, "qwen-plus")
	rep.Advices = buildAdvices(rep, split)
	rep.GeneratedAt = time.Date(2026, 5, 3, 10, 1, 23, 0, time.UTC)

	body := renderDebugDump(rep, split, gc)

	assert.Contains(t, body, "# aicache prompt dump")
	assert.Contains(t, body, "## sections")
	assert.Contains(t, body, "## hit report")
	assert.Contains(t, body, "## advices")
	assert.Contains(t, body, "## raw prompt")
	assert.Contains(t, body, "model:  qwen-plus")
	assert.Contains(t, body, "section=high-static")
	assert.Contains(t, body, "section=semi-dynamic")
	assert.Contains(t, body, "section=timeline")
	assert.Contains(t, body, "section=dynamic")
	assert.Contains(t, body, prompt, "raw prompt body must be embedded verbatim")
}

// 关键词: aicache, debug dump, 落盘文件结构
func TestDumpDebug_WritesFile(t *testing.T) {
	// 临时改写 dump 路径，避免污染默认 YakitBaseTempDir
	t.Setenv("YAKIT_HOME", t.TempDir())

	// 重置 once，保证 resolveDumpBaseDir 重新计算（仅在 test 内可控）
	resetDumpStateForTest()

	gc := newGlobalCache(4)
	prompt := buildFourSectionPrompt("ny", "qy", "tools", "static", "tl", "mem")
	split := Split(prompt)
	rep := gc.Record(split, "test-model")
	rep.Advices = buildAdvices(rep, split)

	dumpDebug(rep, split, gc)

	dir, err := resolveDumpBaseDir()
	require.NoError(t, err)
	require.NotEmpty(t, dir)

	files, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.NotEmpty(t, files)

	target := filepath.Join(dir, files[0].Name())
	data, err := os.ReadFile(target)
	require.NoError(t, err)
	body := string(data)

	assert.True(t, strings.HasPrefix(body, "# aicache prompt dump"))
	assert.Contains(t, body, "model:  test-model")
	assert.Contains(t, body, prompt)
}

// resetDumpStateForTest 重置落盘相关的全局状态，仅供 _test 文件使用
// 关键词: aicache, test helper, reset dump
func resetDumpStateForTest() {
	dumpMu.Lock()
	defer dumpMu.Unlock()
	dumpBaseDir = ""
	dumpBaseDirErr = nil
	dumpSessionId = ""
	// 重置 sync.Once，使用赋零值
	dumpBaseDirOnce = sync.Once{}
}
