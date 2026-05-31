package aibalance

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 关键词: db_mirror_storage_test, 落盘配置单例读写

func TestMirrorStorageConfig_GetDefaultsAndSave(t *testing.T) {
	require.NoError(t, EnsureMirrorStorageConfigTable())

	cfg, err := GetMirrorStorageConfig()
	require.NoError(t, err)
	require.NotNil(t, cfg)
	// 默认值兜底校验。
	assert.Equal(t, defaultDataSinkMaxBytes, cfg.MaxBytes)
	assert.Equal(t, defaultDataSinkReclaimBytes, cfg.ReclaimBytes)
	assert.Equal(t, defaultDataSinkCheckSec, cfg.CheckIntervalSec)

	// 写回新值并读出验证。
	cfg.Enabled = true
	cfg.MaxBytes = 3 << 30
	cfg.ReclaimBytes = 1 << 30
	cfg.CheckIntervalSec = 120
	require.NoError(t, SaveMirrorStorageConfig(cfg))

	got, err := GetMirrorStorageConfig()
	require.NoError(t, err)
	assert.True(t, got.Enabled)
	assert.Equal(t, int64(3<<30), got.MaxBytes)
	assert.Equal(t, int64(1<<30), got.ReclaimBytes)
	assert.Equal(t, int64(120), got.CheckIntervalSec)

	// 还原默认, 避免污染其它用例（落盘默认关闭）。
	got.Enabled = false
	got.MaxBytes = defaultDataSinkMaxBytes
	got.ReclaimBytes = defaultDataSinkReclaimBytes
	got.CheckIntervalSec = defaultDataSinkCheckSec
	require.NoError(t, SaveMirrorStorageConfig(got))
}

func TestMirrorStorageConfig_ZeroValuesFallback(t *testing.T) {
	require.NoError(t, EnsureMirrorStorageConfigTable())

	cfg := &AiMirrorStorageConfig{Enabled: false, MaxBytes: 0, ReclaimBytes: 0, CheckIntervalSec: 0}
	require.NoError(t, SaveMirrorStorageConfig(cfg))

	got, err := GetMirrorStorageConfig()
	require.NoError(t, err)
	assert.Equal(t, defaultDataSinkMaxBytes, got.MaxBytes)
	assert.Equal(t, defaultDataSinkReclaimBytes, got.ReclaimBytes)
	assert.Equal(t, defaultDataSinkCheckSec, got.CheckIntervalSec)
}
