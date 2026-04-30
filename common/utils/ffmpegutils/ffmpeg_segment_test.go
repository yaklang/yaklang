package ffmpegutils

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/log"
)

// 视频切片测试关键词: ExtractVideoSliceFromVideo, segment muxer, smoke test

func TestSmoke_ExtractVideoSlice_StreamCopy(t *testing.T) {
	log.SetLevel(log.DebugLevel)
	videoPath, cleanup := setupTestWithEmbeddedData(t)
	defer cleanup()

	outputDir, err := os.MkdirTemp("", "slice-streamcopy-*")
	assert.NoError(t, err)
	defer os.RemoveAll(outputDir)

	var callbackCount int64
	var lastIdxFromCallback int64 = -1

	// 30 秒/段，72 秒视频应至少切出 3 片（最后一片不足 30s 也算）
	// 关键词: 切片段长, slice duration
	ch, err := ExtractVideoSliceFromVideo(videoPath,
		WithDebug(true),
		WithSliceDurationSeconds(30),
		WithSliceOutputDir(outputDir),
		WithSliceCallback(func(r *VideoSliceResult) {
			atomic.AddInt64(&callbackCount, 1)
			atomic.StoreInt64(&lastIdxFromCallback, int64(r.Index))
			log.Infof("callback received slice idx=%d path=%s size=%d", r.Index, r.FilePath, r.SizeBytes)
		}),
	)
	assert.NoError(t, err)

	var collected []*VideoSliceResult
	timer := time.NewTimer(60 * time.Second)
	defer timer.Stop()

	done := false
	for !done {
		select {
		case r, ok := <-ch:
			if !ok {
				done = true
				break
			}
			collected = append(collected, r)
		case <-timer.C:
			t.Fatalf("video slice timeout after 60s, collected %d slices", len(collected))
		}
	}

	// 验证切片数量
	assert.GreaterOrEqual(t, len(collected), 2, "should produce at least 2 slices for a 72s video at 30s/segment")
	assert.GreaterOrEqual(t, atomic.LoadInt64(&callbackCount), int64(2), "callback should fire for each slice")

	// 验证 channel 与 callback 数量一致（任一成功路径都要 +1）
	successFromChan := 0
	for _, r := range collected {
		if r.Error == nil {
			successFromChan++
		}
	}
	assert.Equal(t, successFromChan, int(atomic.LoadInt64(&callbackCount)), "callback count should match successful channel emissions")

	// 验证文件确实落盘且能播放
	for i, r := range collected {
		if r.Error != nil {
			continue
		}
		assert.Equal(t, i, r.Index, "slice index should monotonically increase from 0")
		assert.Equal(t, "video/mp4", r.MIMEType)
		assert.Greater(t, r.SizeBytes, int64(0))
		stat, err := os.Stat(r.FilePath)
		assert.NoError(t, err)
		assert.Equal(t, r.SizeBytes, stat.Size())
		assert.True(t, strings.HasSuffix(r.FilePath, ".mp4"))
		// 默认不加载 RawData
		assert.Nil(t, r.RawData)
	}
}

func TestSmoke_ExtractVideoSlice_LoadRawData(t *testing.T) {
	log.SetLevel(log.DebugLevel)
	videoPath, cleanup := setupTestWithEmbeddedData(t)
	defer cleanup()

	outputDir, err := os.MkdirTemp("", "slice-rawdata-*")
	assert.NoError(t, err)
	defer os.RemoveAll(outputDir)

	ch, err := ExtractVideoSliceFromVideo(videoPath,
		WithSliceDurationSeconds(30),
		WithSliceOutputDir(outputDir),
		WithSliceLoadRawData(true),
	)
	assert.NoError(t, err)

	got := false
	for r := range ch {
		if r.Error != nil {
			continue
		}
		assert.NotEmpty(t, r.RawData, "RawData should be filled when WithSliceLoadRawData(true)")
		assert.Equal(t, int64(len(r.RawData)), r.SizeBytes)
		got = true
	}
	assert.True(t, got, "should emit at least one slice with raw data")
}

func TestSmoke_ExtractVideoSlice_Reencode(t *testing.T) {
	log.SetLevel(log.DebugLevel)
	if _, err := exec.LookPath("ffprobe"); err != nil {
		t.Skip("ffprobe not available, skip reencode validation")
	}
	videoPath, cleanup := setupTestWithEmbeddedData(t)
	defer cleanup()

	outputDir, err := os.MkdirTemp("", "slice-reencode-*")
	assert.NoError(t, err)
	defer os.RemoveAll(outputDir)

	// 重编码模式: 限制 480p / 2 fps
	// 关键词: 重编码模式验证, scale, target fps
	ch, err := ExtractVideoSliceFromVideo(videoPath,
		WithSliceDurationSeconds(30),
		WithSliceOutputDir(outputDir),
		WithSliceReencode(true),
		WithSliceMaxHeight(480),
		WithSliceTargetFPS(2),
	)
	assert.NoError(t, err)

	var first *VideoSliceResult
	for r := range ch {
		if r.Error != nil {
			continue
		}
		if first == nil {
			first = r
		}
	}
	assert.NotNil(t, first, "should produce at least one slice in reencode mode")

	// 验证分辨率与帧率
	out, err := exec.Command("ffprobe", "-v", "error",
		"-select_streams", "v:0",
		"-show_entries", "stream=width,height,r_frame_rate",
		"-of", "default=noprint_wrappers=1",
		first.FilePath,
	).Output()
	assert.NoError(t, err)
	probe := string(out)
	log.Infof("ffprobe output: %s", probe)
	assert.Contains(t, probe, "height=480")
	// r_frame_rate 输出 "2/1"
	assert.True(t, strings.Contains(probe, "r_frame_rate=2/1") || strings.Contains(probe, "r_frame_rate=2"),
		"expected target fps 2, got: %s", probe)
}

func TestSmoke_ExtractVideoSlice_Preset(t *testing.T) {
	videoPath, cleanup := setupTestWithEmbeddedData(t)
	defer cleanup()

	outputDir, err := os.MkdirTemp("", "slice-preset-*")
	assert.NoError(t, err)
	defer os.RemoveAll(outputDir)

	// turbo preset => 30 秒/段
	// 关键词: omni preset, turbo preset
	ch, err := ExtractVideoSliceFromVideo(videoPath,
		WithSlicePresetForOmni("turbo"),
		WithSliceOutputDir(outputDir),
	)
	assert.NoError(t, err)

	count := 0
	for r := range ch {
		if r.Error == nil {
			count++
		}
	}
	// 72s / 30s ≈ 3 段
	assert.GreaterOrEqual(t, count, 2)

	// 检查输出目录里实际有切片
	entries, err := os.ReadDir(outputDir)
	assert.NoError(t, err)
	mp4Count := 0
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".mp4" {
			mp4Count++
		}
	}
	assert.Equal(t, count, mp4Count)
}
