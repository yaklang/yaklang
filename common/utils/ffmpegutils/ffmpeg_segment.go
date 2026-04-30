package ffmpegutils

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/log"
)

// 视频切片关键词: ExtractVideoSliceFromVideo, ffmpeg segment muxer, 流复制切片
//
// ExtractVideoSliceFromVideo 把输入视频按时间切成若干 mp4 文件并实时下发结果。
//
// 默认走 ffmpeg segment muxer 流复制（-c copy），不重编码，速度极快；
// 当 WithSliceReencode(true) 时会重新编码到指定分辨率与 FPS。
//
// 返回的 channel 会在每个分片落盘后即刻收到 *VideoSliceResult；
// 同时若用户通过 WithSliceCallback 注册了回调，回调也会被同步触发。
//
// example:
//
//	ch, err := ffmpegutils.ExtractVideoSliceFromVideo("input.mp4",
//	    ffmpegutils.WithSlicePresetForOmni("flash"),
//	    ffmpegutils.WithSliceCallback(func(r *VideoSliceResult) {
//	        log.Infof("slice ready: %s (%d bytes)", r.FilePath, r.SizeBytes)
//	    }),
//	)
//	if err != nil { return err }
//	for r := range ch {
//	    if r.Error != nil { continue }
//	    // upload r.FilePath to omni model
//	}
func ExtractVideoSliceFromVideo(inputFile string, opts ...Option) (<-chan *VideoSliceResult, error) {
	if _, err := os.Stat(inputFile); os.IsNotExist(err) {
		return nil, fmt.Errorf("input file does not exist: %s", inputFile)
	}
	if ffmpegBinaryPath == "" {
		return nil, fmt.Errorf("ffmpeg binary path is not configured")
	}

	o := newDefaultOptions()
	for _, opt := range opts {
		opt(o)
	}

	if o.sliceDurationSeconds <= 0 {
		return nil, fmt.Errorf("slice duration must be > 0 seconds")
	}

	// 准备输出目录
	// 关键词: 切片输出目录, output dir
	if o.sliceOutputDir == "" {
		tempDir, err := ioutil.TempDir("", "video-slices-*")
		if err != nil {
			return nil, fmt.Errorf("failed to create temporary slice output directory: %w", err)
		}
		o.sliceOutputDir = tempDir
	} else {
		cleanedPath := filepath.Clean(o.sliceOutputDir)
		if err := os.MkdirAll(cleanedPath, 0750); err != nil {
			return nil, fmt.Errorf("failed to create slice output directory: %w", err)
		}
		o.sliceOutputDir = cleanedPath
	}

	// 构造 ffmpeg 命令参数
	// 关键词: ffmpeg segment muxer, segment_time, reset_timestamps
	outputPattern := filepath.Join(o.sliceOutputDir, "slice_%05d.mp4")
	args := []string{
		"-i", inputFile,
		"-nostdin",
		"-y",
		"-threads", strconv.Itoa(o.threads),
		"-map", "0",
	}

	if o.sliceReencode {
		// 重编码模式: 统一分辨率与 FPS，文件体积更小、token 控制更精准
		// 关键词: 重编码模式, libx264, scale, target fps
		vf := fmt.Sprintf("scale='min(iw,trunc(oh*a/2)*2)':'min(%d,ih)'", o.sliceMaxHeight)
		args = append(args,
			"-c:v", "libx264",
			"-preset", "veryfast",
			"-crf", "26",
			"-pix_fmt", "yuv420p",
			"-r", fmt.Sprintf("%g", o.sliceTargetFPS),
			"-vf", vf,
			"-c:a", "aac",
			"-b:a", "64k",
		)
	} else {
		// 流复制模式: 速度极快，分辨率/FPS 保持源
		// 关键词: 流复制模式, stream copy, c copy
		args = append(args, "-c", "copy")
	}

	args = append(args,
		"-f", "segment",
		"-segment_time", fmt.Sprintf("%g", o.sliceDurationSeconds),
		"-reset_timestamps", "1",
		"-segment_format", "mp4",
		"-movflags", "+faststart",
		outputPattern,
	)

	resultsChan := make(chan *VideoSliceResult, 64)

	go func() {
		defer close(resultsChan)
		runCtx, cancel := context.WithCancel(o.ctx)
		defer cancel()

		// 关键词: ffmpeg 命令执行, segment 命令构造
		cmd := exec.CommandContext(runCtx, ffmpegBinaryPath, args...)
		if o.debug {
			cmd.Stderr = log.NewLogWriter(log.DebugLevel)
		} else {
			cmd.Stderr = ioutil.Discard
		}

		// 轮询输出目录，新文件出现即下发上一个分片
		// 关键词: 轮询切片落盘, slice polling
		processed := make(map[string]bool)
		var pollMu sync.Mutex
		var sliceIndex int
		var emitMu sync.Mutex

		// emit 用于把已经落盘完整的分片下发到 channel + 回调
		// finalFlush 为 true 时表示进入最终刷出阶段，不再检查 runCtx，
		// 用 select-default 安全降级避免消费者已退出时阻塞。
		emit := func(name string, finalFlush bool) {
			emitMu.Lock()
			defer emitMu.Unlock()

			fullPath := filepath.Join(o.sliceOutputDir, name)
			info, statErr := os.Stat(fullPath)
			if statErr != nil {
				log.Warnf("video slice stat failed: %s, err=%v", fullPath, statErr)
				return
			}

			startSec := float64(sliceIndex) * o.sliceDurationSeconds
			endSec := startSec + o.sliceDurationSeconds

			result := &VideoSliceResult{
				FilePath:  fullPath,
				Index:     sliceIndex,
				StartTime: time.Duration(startSec * float64(time.Second)),
				EndTime:   time.Duration(endSec * float64(time.Second)),
				SizeBytes: info.Size(),
				MIMEType:  "video/mp4",
			}

			if o.sliceLoadRawData {
				data, err := ioutil.ReadFile(fullPath)
				if err != nil {
					result.Error = fmt.Errorf("failed to read slice file %s: %w", fullPath, err)
				} else {
					result.RawData = data
				}
			}

			if o.sliceCallback != nil {
				func() {
					defer func() {
						if r := recover(); r != nil {
							log.Errorf("slice callback panic: %v", r)
						}
					}()
					o.sliceCallback(result)
				}()
			}

			if finalFlush {
				// 最终阶段绕开 runCtx 检查；channel 仍未关闭，buffer 64 一般够用
				// 关键词: 最终阶段安全发送, final flush emit
				select {
				case resultsChan <- result:
				default:
					log.Warnf("final flush: channel full or no reader, slice idx=%d skipped on send", result.Index)
				}
			} else {
				select {
				case resultsChan <- result:
				case <-runCtx.Done():
				}
			}

			sliceIndex++
		}

		// scanFlushable 把目前看到的、确认已写完的分片刷出
		// 在 ffmpeg 仍运行时：除最新文件外的全部按字典序刷出（最新文件可能仍在写）
		// 在 ffmpeg 结束后：把所有未刷出的全刷出
		scanFlushable := func(force bool) {
			finalFlush := force
			pollMu.Lock()
			defer pollMu.Unlock()

			entries, err := ioutil.ReadDir(o.sliceOutputDir)
			if err != nil {
				return
			}
			var names []string
			for _, e := range entries {
				if e.IsDir() {
					continue
				}
				name := e.Name()
				if !strings.HasPrefix(name, "slice_") || !strings.HasSuffix(name, ".mp4") {
					continue
				}
				if processed[name] {
					continue
				}
				names = append(names, name)
			}
			if len(names) == 0 {
				return
			}
			sort.Strings(names)

			var toEmit []string
			if force {
				toEmit = names
			} else if len(names) > 1 {
				// 除最末文件外都已稳定
				toEmit = names[:len(names)-1]
			}
			for _, n := range toEmit {
				processed[n] = true
			}
			for _, n := range toEmit {
				emit(n, finalFlush)
			}
		}

		// 启动 poller
		// 关键词: 切片 poller, 200ms 轮询
		pollerStop := make(chan struct{})
		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			ticker := time.NewTicker(200 * time.Millisecond)
			defer ticker.Stop()
			for {
				select {
				case <-runCtx.Done():
					return
				case <-pollerStop:
					return
				case <-ticker.C:
					scanFlushable(false)
				}
			}
		}()

		log.Infof("executing ffmpeg video slice: %s", cmd.String())
		runErr := cmd.Run()
		// ffmpeg 结束后通知 poller 停止，但不要 cancel runCtx，
		// 这样最终 flush 阶段也不会因为 ctx done 而误判
		// 关键词: poller 优雅停止, graceful poller stop
		close(pollerStop)
		wg.Wait()

		// 最终把剩余分片全部刷出（finalFlush=true，绕开 runCtx 检查）
		// 关键词: 最终切片刷出, final flush
		scanFlushable(true)

		if runErr != nil {
			select {
			case resultsChan <- &VideoSliceResult{Error: fmt.Errorf("ffmpeg execution failed: %w", runErr), Index: -1}:
			default:
			}
		}
	}()

	return resultsChan, nil
}
