package aibalance

// data_sink.go - aibalance 镜像数据落盘子系统
//
// 职责: 给镜像回调脚本提供一个 save() 落盘能力, 把镜像数据按天分片追加成 JSONL
// 文件存到用户目录下的 aibalance-data 目录, 便于离线排障与容量统计。
//
// 设计取舍:
//   - 按天分片 (YYYY-MM-DD.jsonl): 旧数据按整文件淘汰, 容量治理只删最旧的整文件。
//   - 写入串行 (mutex + 常开当天文件句柄), 跨天自动滚动新文件。
//   - 计数 (records / bytes) 维护在内存, 落到 sidecar (.index.json) 便于重启快速恢复;
//     sidecar 缺失时后台重算。容量治理 tick 会用目录实际大小重算 bytes (权威)。
//   - 容量上限 / 单次回收量 / 巡检间隔均可配置 (见 AiMirrorStorageConfig)。
//
// 关键词: aibalance dataSink, 镜像数据落盘, JSONL 按天分片, 容量治理, sidecar 计数

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
)

const (
	// dataSinkDirName 是落盘根目录名 (位于用户 home 目录下)。
	dataSinkDirName = "aibalance-data"

	// dataSinkIndexFile 是计数 sidecar 文件名。
	dataSinkIndexFile = ".index.json"

	// dataSinkFileSuffix 是分片文件后缀。
	dataSinkFileSuffix = ".jsonl"

	// 默认容量参数 (字节)。1 GiB = 1<<30。
	defaultDataSinkMaxBytes     int64 = 6 << 30 // 6 GiB 上限
	defaultDataSinkReclaimBytes int64 = 2 << 30 // 超限时单次至少回收 2 GiB
	defaultDataSinkCheckSec     int64 = 60      // 每分钟巡检一次

	// recentRecords 读取分片尾部时的最大回扫字节, 防止单条超大记录把内存撑爆。
	dataSinkTailScanCap int64 = 16 << 20 // 16 MiB
)

// dataSink 是落盘子系统的运行时态。
//
// 关键词: dataSink struct, 串行追加 + 计数 + 容量治理
type dataSink struct {
	mu  sync.Mutex
	dir string

	enabled      bool
	maxBytes     int64
	reclaimBytes int64
	checkSec     int64

	curDate string
	curFile *os.File

	records int64
	bytes   int64
	counted bool // records/bytes 是否已从磁盘初始化

	lastIndexFlush time.Time
}

// dataSinkIndex 是 sidecar 的 JSON 结构。
type dataSinkIndex struct {
	Records   int64  `json:"records"`
	Bytes     int64  `json:"bytes"`
	UpdatedAt string `json:"updated_at"`
}

// 全局单例。balancer 启动时通过 initDataSink 装配; 未装配前所有调用安全 no-op。
var (
	globalDataSink   *dataSink
	globalDataSinkMu sync.RWMutex
)

// resolveDataSinkDir 解析落盘根目录: 优先用户 home 目录, 失败回落 yakit base dir。
// 关键词: resolveDataSinkDir, os.UserHomeDir, consts.GetDefaultYakitBaseDir 回落
func resolveDataSinkDir() string {
	if home, err := os.UserHomeDir(); err == nil && strings.TrimSpace(home) != "" {
		return filepath.Join(home, dataSinkDirName)
	}
	base := consts.GetDefaultYakitBaseDir()
	if strings.TrimSpace(base) == "" {
		base = os.TempDir()
	}
	return filepath.Join(base, dataSinkDirName)
}

// newDataSink 构造一个落盘器 (不创建目录, 首次写入时再 MkdirAll)。
func newDataSink(dir string) *dataSink {
	return &dataSink{
		dir:          dir,
		enabled:      false,
		maxBytes:     defaultDataSinkMaxBytes,
		reclaimBytes: defaultDataSinkReclaimBytes,
		checkSec:     defaultDataSinkCheckSec,
	}
}

// applyConfig 热更新配置 (启用开关 / 容量参数)。<=0 的容量参数回落默认值。
// 关键词: dataSink.applyConfig, 热更新容量参数
func (s *dataSink) applyConfig(enabled bool, maxBytes, reclaimBytes, checkSec int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.enabled = enabled
	if maxBytes > 0 {
		s.maxBytes = maxBytes
	} else {
		s.maxBytes = defaultDataSinkMaxBytes
	}
	if reclaimBytes > 0 {
		s.reclaimBytes = reclaimBytes
	} else {
		s.reclaimBytes = defaultDataSinkReclaimBytes
	}
	if checkSec > 0 {
		s.checkSec = checkSec
	} else {
		s.checkSec = defaultDataSinkCheckSec
	}
}

// shardName 返回某天的分片文件名 (相对 dir)。
func shardName(date string) string {
	return date + dataSinkFileSuffix
}

// nowDate 返回本地当天日期 (YYYY-MM-DD)。落盘按本地日切片, 与文件名可读性对齐。
func nowDate() string {
	return time.Now().Format("2006-01-02")
}

// rotateLocked 确保当天分片文件句柄已就绪 (调用方需持锁)。
func (s *dataSink) rotateLocked() error {
	date := nowDate()
	if s.curFile != nil && s.curDate == date {
		return nil
	}
	if s.curFile != nil {
		_ = s.curFile.Close()
		s.curFile = nil
	}
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return fmt.Errorf("mkdir data dir failed: %w", err)
	}
	path := filepath.Join(s.dir, shardName(date))
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("open shard failed: %w", err)
	}
	s.curFile = f
	s.curDate = date
	return nil
}

// appendRecord 把一条记录序列化成单行 JSON 追加到当天分片。
// 返回 (written, error)。enabled=false / sink 未装配时不写, 返回 (false, nil)。
// 关键词: dataSink.appendRecord, JSONL 追加, 内存计数
func (s *dataSink) appendRecord(obj any) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.enabled {
		return false, nil
	}
	raw, err := json.Marshal(obj)
	if err != nil {
		return false, fmt.Errorf("marshal record failed: %w", err)
	}
	// 单行 JSON: 去掉内部换行风险后追加一个换行。
	line := append(raw, '\n')
	if err := s.rotateLocked(); err != nil {
		return false, err
	}
	n, werr := s.curFile.Write(line)
	if werr != nil {
		return false, fmt.Errorf("write record failed: %w", werr)
	}
	s.records++
	s.bytes += int64(n)
	s.maybeFlushIndexLocked(false)
	return true, nil
}

// stats 返回当前内存计数 (records, bytes)。
func (s *dataSink) stats() (int64, int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.records, s.bytes
}

// maybeFlushIndexLocked 把计数写到 sidecar; force=false 时按时间去抖 (至少间隔 5s)。
// 关键词: maybeFlushIndexLocked, sidecar 去抖落盘
func (s *dataSink) maybeFlushIndexLocked(force bool) {
	if !force && time.Since(s.lastIndexFlush) < 5*time.Second {
		return
	}
	idx := dataSinkIndex{
		Records:   s.records,
		Bytes:     s.bytes,
		UpdatedAt: time.Now().Format(time.RFC3339),
	}
	raw, err := json.Marshal(&idx)
	if err != nil {
		return
	}
	path := filepath.Join(s.dir, dataSinkIndexFile)
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		log.Warnf("data sink: flush index failed: %v", err)
		return
	}
	s.lastIndexFlush = time.Now()
}

// listShardsLocked 返回目录下所有分片文件名 (按文件名升序, 即按日期从旧到新)。
func (s *dataSink) listShardsLocked() ([]string, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasSuffix(name, dataSinkFileSuffix) {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names, nil
}

// recentRecords 读取最近 n 条记录 (newest-first), 跨分片回溯, 解析为 map。
// 关键词: dataSink.recentRecords, 分片尾部回扫, newest-first
func (s *dataSink) recentRecords(n int) ([]map[string]any, error) {
	if n <= 0 {
		n = 20
	}
	s.mu.Lock()
	shards, err := s.listShardsLocked()
	dir := s.dir
	s.mu.Unlock()
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, n)
	// 从最新分片开始往旧回溯。
	for i := len(shards) - 1; i >= 0 && len(out) < n; i-- {
		lines, lerr := readLastJSONLines(filepath.Join(dir, shards[i]), n-len(out))
		if lerr != nil {
			log.Warnf("data sink: read shard %s failed: %v", shards[i], lerr)
			continue
		}
		// lines 已是该文件内 newest-last 的顺序的「最后若干行」, 逐行从尾到头解析。
		for j := len(lines) - 1; j >= 0 && len(out) < n; j-- {
			var m map[string]any
			if jerr := json.Unmarshal(lines[j], &m); jerr != nil {
				continue
			}
			out = append(out, m)
		}
	}
	return out, nil
}

// readLastJSONLines 读取文件末尾的最后 n 个非空行 (保持原始先后顺序, 即返回切片
// 最后一个元素是文件最新一行)。从文件尾部回扫, 最多扫 dataSinkTailScanCap 字节。
// 关键词: readLastJSONLines, 文件尾部回扫, 不全量读取大文件
func readLastJSONLines(path string, n int) ([][]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return nil, err
	}
	size := fi.Size()
	if size == 0 {
		return nil, nil
	}
	var scan int64 = 64 << 10
	for {
		if scan > size {
			scan = size
		}
		buf := make([]byte, scan)
		if _, rerr := f.ReadAt(buf, size-scan); rerr != nil && rerr != io.EOF {
			return nil, rerr
		}
		lines := splitNonEmptyLines(buf)
		// 若没读到文件头部, 第一行可能是被截断的半行, 丢弃。
		if scan < size && len(lines) > 0 {
			lines = lines[1:]
		}
		if int64(len(lines)) >= int64(n) || scan >= size || scan >= dataSinkTailScanCap {
			if len(lines) > n {
				lines = lines[len(lines)-n:]
			}
			return lines, nil
		}
		scan *= 2
	}
}

// splitNonEmptyLines 把字节切片按 \n 拆成非空行 (去掉 \r), 保持顺序。
func splitNonEmptyLines(buf []byte) [][]byte {
	parts := strings.Split(string(buf), "\n")
	out := make([][]byte, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimRight(p, "\r")
		if strings.TrimSpace(p) == "" {
			continue
		}
		out = append(out, []byte(p))
	}
	return out
}

// dirSizeLocked 走查目录求所有分片总字节 (不含 sidecar)。
func (s *dataSink) dirSizeLocked() (int64, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	var total int64
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), dataSinkFileSuffix) {
			continue
		}
		if info, ierr := e.Info(); ierr == nil {
			total += info.Size()
		}
	}
	return total, nil
}

// enforceCapacity 巡检目录总大小, 超过 maxBytes 时从最旧分片开始整文件删除,
// 直到至少回收 reclaimBytes (或只剩当天文件)。删除前先数该文件行数以修正 records。
// 删除完毕后用目录实际大小重算 bytes (权威), 并落 sidecar。
// 关键词: dataSink.enforceCapacity, 超限删最旧, 行数修正计数, 目录实际大小重算
func (s *dataSink) enforceCapacity() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	total, err := s.dirSizeLocked()
	if err != nil {
		return err
	}
	// 即便没超限, 也借机把 bytes 校准成目录实际大小 (修正 append 时的近似)。
	s.bytes = total
	if total <= s.maxBytes {
		s.maybeFlushIndexLocked(true)
		return nil
	}

	shards, err := s.listShardsLocked()
	if err != nil {
		return err
	}
	today := nowDate()
	var reclaimed int64
	for _, name := range shards {
		if reclaimed >= s.reclaimBytes {
			break
		}
		// 保护当天分片, 避免把正在写的文件删掉。
		if name == shardName(today) {
			continue
		}
		path := filepath.Join(s.dir, name)
		info, ierr := os.Stat(path)
		if ierr != nil {
			continue
		}
		fileLines := countFileLines(path)
		if rmErr := os.Remove(path); rmErr != nil {
			log.Warnf("data sink: remove old shard %s failed: %v", name, rmErr)
			continue
		}
		reclaimed += info.Size()
		s.records -= fileLines
		if s.records < 0 {
			s.records = 0
		}
		log.Infof("data sink: reclaimed old shard %s (size=%d lines=%d)", name, info.Size(), fileLines)
	}

	// 重算目录实际大小作为权威 bytes。
	if newTotal, e2 := s.dirSizeLocked(); e2 == nil {
		s.bytes = newTotal
	}
	s.maybeFlushIndexLocked(true)
	log.Infof("data sink: capacity governor done, reclaimed=%d bytes, now=%d/%d", reclaimed, s.bytes, s.maxBytes)
	return nil
}

// countFileLines 数文件非空行数 (用于删除时修正 records)。失败返回 0。
func countFileLines(path string) int64 {
	f, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer f.Close()
	var count int64
	buf := make([]byte, 256<<10)
	var pendingNonEmpty bool
	for {
		n, rerr := f.Read(buf)
		for i := 0; i < n; i++ {
			if buf[i] == '\n' {
				if pendingNonEmpty {
					count++
				}
				pendingNonEmpty = false
			} else if buf[i] != '\r' && buf[i] != ' ' && buf[i] != '\t' {
				pendingNonEmpty = true
			}
		}
		if rerr != nil {
			break
		}
	}
	if pendingNonEmpty {
		count++
	}
	return count
}

// initCountFromDisk 初始化内存计数: 优先读 sidecar; 缺失/损坏则全量重算 (行数 + 目录大小)。
// 全量重算可能较慢, 调用方应放后台 goroutine。
// 关键词: dataSink.initCountFromDisk, sidecar 优先, 缺失则重算
func (s *dataSink) initCountFromDisk() {
	s.mu.Lock()
	if s.counted {
		s.mu.Unlock()
		return
	}
	dir := s.dir
	s.mu.Unlock()

	// 1. 试读 sidecar。
	if raw, err := os.ReadFile(filepath.Join(dir, dataSinkIndexFile)); err == nil {
		var idx dataSinkIndex
		if json.Unmarshal(raw, &idx) == nil && idx.Records >= 0 {
			s.mu.Lock()
			s.records = idx.Records
			s.bytes = idx.Bytes
			s.counted = true
			s.mu.Unlock()
			// 顺手用目录实际大小校准一次 bytes。
			if total, e := func() (int64, error) { s.mu.Lock(); defer s.mu.Unlock(); return s.dirSizeLocked() }(); e == nil {
				s.mu.Lock()
				s.bytes = total
				s.mu.Unlock()
			}
			return
		}
	}

	// 2. sidecar 缺失/损坏: 全量重算行数与大小。
	shards, err := s.listShardsLocked2()
	if err != nil {
		log.Warnf("data sink: recount list shards failed: %v", err)
		return
	}
	var records, total int64
	for _, name := range shards {
		path := filepath.Join(dir, name)
		records += countFileLines(path)
		if info, ierr := os.Stat(path); ierr == nil {
			total += info.Size()
		}
	}
	s.mu.Lock()
	s.records = records
	s.bytes = total
	s.counted = true
	s.maybeFlushIndexLocked(true)
	s.mu.Unlock()
	log.Infof("data sink: recounted from disk records=%d bytes=%d", records, total)
}

// listShardsLocked2 是 listShardsLocked 的自加锁版本 (initCountFromDisk 后台用)。
func (s *dataSink) listShardsLocked2() ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.listShardsLocked()
}

// ==================== 全局封装 ====================

// initDataSink 装配全局单例并按配置热更新; 后台初始化计数。
// 关键词: initDataSink, 全局落盘器装配
func initDataSink(enabled bool, maxBytes, reclaimBytes, checkSec int64) {
	globalDataSinkMu.Lock()
	if globalDataSink == nil {
		globalDataSink = newDataSink(resolveDataSinkDir())
	}
	sink := globalDataSink
	globalDataSinkMu.Unlock()

	sink.applyConfig(enabled, maxBytes, reclaimBytes, checkSec)
	go sink.initCountFromDisk()
}

// getDataSink 返回全局单例 (可能为 nil)。
func getDataSink() *dataSink {
	globalDataSinkMu.RLock()
	defer globalDataSinkMu.RUnlock()
	return globalDataSink
}

// dataSinkAppend 给镜像 save() 用: 落盘一条记录。sink 未装配/未启用时安全 no-op。
// 关键词: dataSinkAppend, save 落盘入口
func dataSinkAppend(obj any) (bool, error) {
	sink := getDataSink()
	if sink == nil {
		return false, nil
	}
	return sink.appendRecord(obj)
}

// dataSinkEnabled 返回落盘是否处于启用状态（用于试运行预判生产是否会真正落盘）。
// 未装配 / 未启用都返回 false。
// 关键词: dataSinkEnabled, 落盘启用状态预判
func dataSinkEnabled() bool {
	sink := getDataSink()
	if sink == nil {
		return false
	}
	sink.mu.Lock()
	defer sink.mu.Unlock()
	return sink.enabled
}

// dataSinkStats 返回 (records, bytes, available)。未装配时 available=false。
// 关键词: dataSinkStats, 面板计数
func dataSinkStats() (int64, int64, bool) {
	sink := getDataSink()
	if sink == nil {
		return 0, 0, false
	}
	r, b := sink.stats()
	return r, b, true
}

// dataSinkRecent 返回最近 n 条记录 (newest-first)。未装配返回空。
// 关键词: dataSinkRecent, 最近记录查看
func dataSinkRecent(n int) ([]map[string]any, error) {
	sink := getDataSink()
	if sink == nil {
		return []map[string]any{}, nil
	}
	return sink.recentRecords(n)
}

// StartDataSinkGovernor 启动后台容量巡检 goroutine, 按配置间隔 (默认 60s) 周期执行
// enforceCapacity。ctx.Done 退出。
// 关键词: StartDataSinkGovernor, 每分钟容量巡检调度
func StartDataSinkGovernor(ctx context.Context) {
	go func() {
		log.Infof("data sink governor started")
		for {
			sink := getDataSink()
			interval := defaultDataSinkCheckSec
			if sink != nil {
				sink.mu.Lock()
				interval = sink.checkSec
				sink.mu.Unlock()
			}
			if interval <= 0 {
				interval = defaultDataSinkCheckSec
			}
			select {
			case <-ctx.Done():
				log.Infof("data sink governor stopped")
				return
			case <-time.After(time.Duration(interval) * time.Second):
				if sink := getDataSink(); sink != nil {
					if err := sink.enforceCapacity(); err != nil {
						log.Warnf("data sink: enforce capacity failed: %v", err)
					}
				}
			}
		}
	}()
}
