package diagnostics

import (
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

type Measurement struct {
	Name       string
	Total      time.Duration
	Min        time.Duration
	Max        time.Duration
	Count      uint64
	ErrorCount uint64
	Steps      []time.Duration
	Size       int64 // 文件大小（字节），0 表示未知，用于计算 ms/KB 比例
}

func (m Measurement) Average() time.Duration {
	if m.Count == 0 {
		return 0
	}
	return m.Total / time.Duration(m.Count)
}

func (m Measurement) String() string {
	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("----------- Measurement [%s] --------------------\n", m.Name))
	builder.WriteString(fmt.Sprintf("-------- Measurement %s\tCount %v\n", m.Name, m.Count))
	if m.Count == 0 {
		return builder.String()
	}

	builder.WriteString(fmt.Sprintf("%s--all\tTime: %v\tCount: %v\tAvg: %v\n",
		m.Name, m.Total, m.Count, m.Average(),
	))
	builder.WriteString(fmt.Sprintf("%s--range\tMin: %v\tMax: %v\n",
		m.Name, m.Min, m.Max,
	))

	if m.Count > 1 {
		for index, t := range m.Steps {
			stepAvg := time.Duration(0)
			if m.Count > 0 {
				stepAvg = t / time.Duration(m.Count)
			}
			builder.WriteString(fmt.Sprintf("%s-%-4d\tTime: %v\tCount: %v\tAvg: %v\n",
				m.Name, index+1, t, m.Count, stepAvg,
			))
		}
	}
	return builder.String()
}

type measurementData struct {
	mu          sync.Mutex
	stepCap     uint32
	measurement Measurement
}

func newMeasurementData(name string, stepCapacity int) *measurementData {
	steps := make([]time.Duration, stepCapacity)
	return &measurementData{
		stepCap: uint32(stepCapacity),
		measurement: Measurement{
			Name:       name,
			Steps:      steps,
			Total:      0,
			Min:        0,
			Max:        0,
			Count:      0,
			ErrorCount: 0,
		},
	}
}

func (m *measurementData) ensureStepCapacity(count int) {
	if count <= 0 {
		return
	}
	if count <= int(atomic.LoadUint32(&m.stepCap)) {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if count <= len(m.measurement.Steps) {
		atomic.StoreUint32(&m.stepCap, uint32(len(m.measurement.Steps)))
		return
	}
	newSteps := make([]time.Duration, count)
	copy(newSteps, m.measurement.Steps)
	m.measurement.Steps = newSteps
	atomic.StoreUint32(&m.stepCap, uint32(count))
}

func (m *measurementData) record(total time.Duration, stepDurations []time.Duration) {
	m.recordWithSize(total, stepDurations, 0)
}

func (m *measurementData) recordWithSize(total time.Duration, stepDurations []time.Duration, size int64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(stepDurations) > len(m.measurement.Steps) {
		newSteps := make([]time.Duration, len(stepDurations))
		copy(newSteps, m.measurement.Steps)
		m.measurement.Steps = newSteps
		atomic.StoreUint32(&m.stepCap, uint32(len(newSteps)))
	}
	for i, dur := range stepDurations {
		m.measurement.Steps[i] += dur
	}

	if m.measurement.Count == 0 {
		m.measurement.Min = total
		m.measurement.Max = total
	} else {
		if total < m.measurement.Min {
			m.measurement.Min = total
		}
		if total > m.measurement.Max {
			m.measurement.Max = total
		}
	}
	m.measurement.Total += total
	m.measurement.Count++
	if size > 0 {
		m.measurement.Size = size
	}
}

func (m *measurementData) markError() {
	m.mu.Lock()
	m.measurement.ErrorCount++
	m.mu.Unlock()
}

func (m *measurementData) snapshot() Measurement {
	m.mu.Lock()
	defer m.mu.Unlock()

	steps := make([]time.Duration, len(m.measurement.Steps))
	copy(steps, m.measurement.Steps)
	return Measurement{
		Name:       m.measurement.Name,
		Total:      m.measurement.Total,
		Min:        m.measurement.Min,
		Max:        m.measurement.Max,
		Count:      m.measurement.Count,
		ErrorCount: m.measurement.ErrorCount,
		Steps:      steps,
		Size:       m.measurement.Size,
	}
}

type Recorder struct {
	entries *utils.SafeMap[*measurementData]
}

func NewRecorder() *Recorder {
	return &Recorder{entries: utils.NewSafeMap[*measurementData]()}
}

func (r *Recorder) ensureEntry(name string, stepCount int) (*measurementData, error) {
	if name == "" {
		return nil, errors.New("diagnostics: measurement name is empty")
	}
	if r == nil {
		return nil, nil
	}
	entry, err := r.entries.GetOrLoad(name, func() (*measurementData, error) {
		return newMeasurementData(name, stepCount), nil
	})
	if err != nil {
		return nil, err
	}
	entry.ensureStepCapacity(stepCount)
	return entry, nil
}

func (r *Recorder) track(enabled bool, name string, steps ...func() error) error {
	if name == "" {
		return errors.New("diagnostics: measurement name is empty")
	}

	if !enabled || r == nil {
		return runStepsWithoutRecording(steps)
	}

	entry, err := r.ensureEntry(name, len(steps))
	if err != nil {
		return err
	}
	if entry == nil {
		return nil
	}

	durations := make([]time.Duration, len(steps))
	var total time.Duration
	for i, step := range steps {
		if step == nil {
			continue
		}
		start := time.Now()
		if err := step(); err != nil {
			entry.markError()
			return err
		}
		elapsed := time.Since(start)
		durations[i] = elapsed
		total += elapsed
	}

	entry.record(total, durations)
	return nil
}

func runStepsWithoutRecording(steps []func() error) error {
	for _, step := range steps {
		if step == nil {
			continue
		}
		if err := step(); err != nil {
			return err
		}
	}
	return nil
}

func (r *Recorder) Snapshot() []Measurement {
	if r == nil {
		return nil
	}
	values := r.entries.Values()
	result := make([]Measurement, 0, len(values))
	for _, entry := range values {
		result = append(result, entry.snapshot())
	}
	slices.SortFunc(result, func(a, b Measurement) int {
		ratioA := 0.0
		if a.Size > 0 && a.Total > 0 {
			ratioA = float64(a.Total.Milliseconds()) / (float64(a.Size) / 1024)
		}
		ratioB := 0.0
		if b.Size > 0 && b.Total > 0 {
			ratioB = float64(b.Total.Milliseconds()) / (float64(b.Size) / 1024)
		}
		switch {
		case ratioA > ratioB:
			return -1 // a 在前（ms/KB 高到低）
		case ratioA < ratioB:
			return 1
		default:
			// ratio 相同，按 Total 降序，再按 Name
			if a.Total != b.Total {
				if a.Total > b.Total {
					return -1
				}
				return 1
			}
			return strings.Compare(a.Name, b.Name)
		}
	})
	return result
}

func (r *Recorder) Reset() {
	if r == nil {
		return
	}
	r.entries = utils.NewSafeMap[*measurementData]()
}

// RecordDuration 记录已经测量的时间（用于外部已经测量好的时间）
func (r *Recorder) RecordDuration(name string, duration time.Duration) {
	r.RecordDurationWithSize(name, duration, 0)
}

// RecordDurationWithSize 记录已测量的时间和文件大小（用于文件级性能日志，可计算 ms/KB）
func (r *Recorder) RecordDurationWithSize(name string, duration time.Duration, fileSize int64) {
	if r == nil || name == "" {
		return
	}
	entry, err := r.ensureEntry(name, 1)
	if err != nil || entry == nil {
		return
	}
	entry.recordWithSize(duration, []time.Duration{duration}, fileSize)
}

func LogRecorder(label string, recorders ...*Recorder) {
	if len(recorders) == 0 {
		recorders = []*Recorder{DefaultRecorder()}
	}
	for _, rec := range recorders {
		if rec != nil {
			rec.Log(label)
		} else {
			log.Warnf("recorder is nil for label: %s", label)
		}
	}
}

func (rec *Recorder) Log(label ...string) {
	if rec == nil {
		log.Infof("recorder %s is nil", label)
		return
	}
	snapshots := rec.Snapshot()
	if len(snapshots) == 0 {
		log.Infof("recorder %s is empty", label)
		return
	}
	// 使用 log.Info 而不是 log.Infof，确保性能日志总是输出
	log.Info("========================================")
	if len(label) > 0 {
		log.Infof("Measurement Summary [%s]", label[0])
	} else {
		log.Info("Measurement Summary")
	}
	log.Info("========================================")
	for _, snapshot := range snapshots {
		log.Info(snapshot.String())
	}
	log.Info("========================================")
}

func CompareRecorderCosts(database, memory *Recorder) {
	if database == nil {
		return
	}
	databaseSnapshots := database.Snapshot()
	memorySnapshots := memory.Snapshot()
	memoryIndex := make(map[string]Measurement, len(memorySnapshots))
	for _, snapshot := range memorySnapshots {
		memoryIndex[snapshot.Name] = snapshot
	}

	for _, databaseMeasurement := range databaseSnapshots {
		memoryMeasurement, ok := memoryIndex[databaseMeasurement.Name]
		if !ok {
			log.Debugf("Measurement [%s] not found in memory cost", databaseMeasurement.Name)
			log.Debug(databaseMeasurement.String())
			continue
		}

		if memoryMeasurement.Count == 0 {
			memoryMeasurement.Count = 1
		}
		if databaseMeasurement.Count > memoryMeasurement.Count*5 {
			log.Debugf("Measurement [%s] count mismatch: database %d, memory %d", databaseMeasurement.Name, databaseMeasurement.Count, memoryMeasurement.Count)
			log.Debug(databaseMeasurement.String())
			log.Debug(memoryMeasurement.String())
		}

		if databaseMeasurement.Total > memoryMeasurement.Total*2 {
			log.Debugf("------------------------------------------------------")
			log.Debugf("Measurement [%s] total time mismatch: database %v, memory %v", databaseMeasurement.Name, databaseMeasurement.Total, memoryMeasurement.Total)
			for index, databaseTime := range databaseMeasurement.Steps {
				if index >= len(memoryMeasurement.Steps) {
					log.Debugf("Measurement %s time mismatch at index %d: database %v, memory not found", databaseMeasurement.Name, index, databaseTime)
					log.Debugf("%s-%-4d\t database Time: %v\tCount: %v\tAvg: %v",
						databaseMeasurement.Name, index+1,
						databaseTime,
						databaseMeasurement.Count,
						databaseTime/time.Duration(databaseMeasurement.Count),
					)
					continue
				}

				memoryTime := memoryMeasurement.Steps[index]
				if databaseTime > memoryTime*2 || databaseTime > time.Second {
					log.Debugf("Measurement %s time mismatch at index %d: database %v, memory %v", databaseMeasurement.Name, index, databaseTime, memoryTime)
					log.Debugf("%s-%-4d\t database Time: %v\tCount: %v\tAvg: %v",
						databaseMeasurement.Name, index+1,
						databaseTime,
						databaseMeasurement.Count,
						databaseTime/time.Duration(databaseMeasurement.Count),
					)
					log.Debugf("%s-%-4d\t memory  Time: %v\tCount: %v\tAvg: %v",
						databaseMeasurement.Name, index+1,
						memoryTime,
						memoryMeasurement.Count,
						memoryTime/time.Duration(memoryMeasurement.Count),
					)
				}
			}
		}
	}
}

// formatSize 格式化文件大小，如 12.3KB、1.2MB
func formatSize(bytes int64) string {
	if bytes <= 0 {
		return "-"
	}
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%dB", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f%cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// formatDuration 格式化耗时：0 显示 "0s"，< 1ms 显示 "xxxµs"，否则使用默认 String()
func formatDuration(d time.Duration) string {
	if d == 0 {
		return "0s"
	}
	if d < time.Millisecond {
		return fmt.Sprintf("%dµs", d.Microseconds())
	}
	return d.String()
}

const (
	perfPrefixBuild     = "Build["
	perfPrefixLazyBuild = "LazyBuild["
	perfSuffixBracket   = "]"
)

// extractFileFromPerfName 从 "Build[filename]" 或 "LazyBuild[filename]" 提取 filename
func extractFileFromPerfName(name, prefix string) string {
	if !strings.HasPrefix(name, prefix) || !strings.HasSuffix(name, perfSuffixBracket) {
		return ""
	}
	return name[len(prefix) : len(name)-1]
}

// MergeBuildAndLazyBuildForDisplay 将 Build[filename] 与 LazyBuild[filename] 合并为一行，Total 相加，用于文件编译性能展示
func MergeBuildAndLazyBuildForDisplay(measurements []Measurement) []Measurement {
	buildByFile := make(map[string]Measurement)
	lazyByFile := make(map[string]Measurement)
	var other []Measurement

	for _, m := range measurements {
		if f := extractFileFromPerfName(m.Name, perfPrefixBuild); f != "" {
			buildByFile[f] = m
			continue
		}
		if f := extractFileFromPerfName(m.Name, perfPrefixLazyBuild); f != "" {
			lazyByFile[f] = m
			continue
		}
		other = append(other, m)
	}

	out := make([]Measurement, 0, len(other)+len(buildByFile)+len(lazyByFile))
	out = append(out, other...)
	for f, b := range buildByFile {
		if l, ok := lazyByFile[f]; ok {
			b.Total += l.Total
			delete(lazyByFile, f)
		}
		out = append(out, b)
	}
	for _, l := range lazyByFile {
		out = append(out, l)
	}
	// 按 ms/KB 降序排序（Size=0 的条目排在最后）
	sort.Slice(out, func(i, j int) bool {
		ri := 0.0
		if out[i].Size > 0 && out[i].Total > 0 {
			ri = float64(out[i].Total.Milliseconds()) / (float64(out[i].Size) / 1024)
		}
		rj := 0.0
		if out[j].Size > 0 && out[j].Total > 0 {
			rj = float64(out[j].Total.Milliseconds()) / (float64(out[j].Size) / 1024)
		}
		if ri != rj {
			return ri > rj
		}
		// ms/KB 相同时按 Duration 降序
		return out[i].Total > out[j].Total
	})
	return out
}

// FormatPerformanceTable 格式化性能数据为表格；若有 Size 则显示文件大小和 ms/KB 比例
func FormatPerformanceTable(title string, measurements []Measurement) string {
	if len(measurements) == 0 {
		return fmt.Sprintf("No performance data for: %s", title)
	}

	hasSize := false
	for _, m := range measurements {
		if m.Size > 0 {
			hasSize = true
			break
		}
	}

	// 计算列宽
	maxNameLen := len("Name")
	maxTimeLen := len("Duration")
	maxSizeLen := len("Size")
	maxRatioLen := len("ms/KB")

	for _, m := range measurements {
		nameLen := len(m.Name)
		if nameLen > 80 {
			nameLen = 80
		}
		if nameLen > maxNameLen {
			maxNameLen = nameLen
		}
		if len(formatDuration(m.Total)) > maxTimeLen {
			maxTimeLen = len(formatDuration(m.Total))
		}
		if hasSize {
			sz := formatSize(m.Size)
			if len(sz) > maxSizeLen {
				maxSizeLen = len(sz)
			}
			if m.Size > 0 && m.Total > 0 {
				ratio := float64(m.Total.Milliseconds()) / (float64(m.Size) / 1024)
				ratioStr := fmt.Sprintf("%.2f", ratio)
				if len(ratioStr) > maxRatioLen {
					maxRatioLen = len(ratioStr)
				}
			}
		}
	}

	if maxNameLen < 30 {
		maxNameLen = 30
	}
	if maxTimeLen < 10 {
		maxTimeLen = 10
	}
	if maxSizeLen < 6 {
		maxSizeLen = 6
	}
	if maxRatioLen < 8 {
		maxRatioLen = 8
	}

	var builder strings.Builder

	totalWidth := maxNameLen + maxTimeLen + 7
	if hasSize {
		totalWidth += maxSizeLen + maxRatioLen + 6
	}
	titleBorder := strings.Repeat("=", totalWidth)
	builder.WriteString(titleBorder + "\n")
	builder.WriteString(fmt.Sprintf(" %s\n", title))
	builder.WriteString(titleBorder + "\n")

	if hasSize {
		headerBorder := fmt.Sprintf("+-%s-+-%s-+-%s-+-%s-+\n",
			strings.Repeat("-", maxNameLen),
			strings.Repeat("-", maxTimeLen),
			strings.Repeat("-", maxSizeLen),
			strings.Repeat("-", maxRatioLen),
		)
		builder.WriteString(headerBorder)
		builder.WriteString(fmt.Sprintf("| %-*s | %*s | %*s | %*s |\n",
			maxNameLen, "Name",
			maxTimeLen, "Duration",
			maxSizeLen, "Size",
			maxRatioLen, "ms/KB",
		))
		builder.WriteString(headerBorder)

		for _, m := range measurements {
			displayName := m.Name
			if len(displayName) > 80 {
				displayName = displayName[:77] + "..."
			}
			sz := formatSize(m.Size)
			ratioStr := "-"
			if m.Size > 0 && m.Total > 0 {
				ratio := float64(m.Total.Milliseconds()) / (float64(m.Size) / 1024)
				ratioStr = fmt.Sprintf("%.2f", ratio)
			}
			builder.WriteString(fmt.Sprintf("| %-*s | %*s | %*s | %*s |\n",
				maxNameLen, displayName,
				maxTimeLen, formatDuration(m.Total),
				maxSizeLen, sz,
				maxRatioLen, ratioStr,
			))
		}
		builder.WriteString(headerBorder)
	} else {
		headerBorder := fmt.Sprintf("+-%s-+-%s-+\n",
			strings.Repeat("-", maxNameLen),
			strings.Repeat("-", maxTimeLen),
		)
		builder.WriteString(headerBorder)
		builder.WriteString(fmt.Sprintf("| %-*s | %*s |\n",
			maxNameLen, "Name",
			maxTimeLen, "Duration",
		))
		builder.WriteString(headerBorder)

		for _, m := range measurements {
			displayName := m.Name
			if len(displayName) > 80 {
				displayName = displayName[:77] + "..."
			}
			builder.WriteString(fmt.Sprintf("| %-*s | %*s |\n",
				maxNameLen, displayName,
				maxTimeLen, formatDuration(m.Total),
			))
		}
		builder.WriteString(headerBorder)
	}
	return builder.String()
}

// FormatSimpleTable 格式化简单的两列表格
func FormatSimpleTable(title string, data map[string]string) string {
	if len(data) == 0 {
		return fmt.Sprintf("No data for: %s", title)
	}

	// 计算列宽
	maxKeyLen := len("Key")
	maxValueLen := len("Value")

	for k, v := range data {
		if len(k) > maxKeyLen {
			maxKeyLen = len(k)
		}
		if len(v) > maxValueLen {
			maxValueLen = len(v)
		}
	}

	// 确保最小宽度
	if maxKeyLen < 20 {
		maxKeyLen = 20
	}
	if maxValueLen < 20 {
		maxValueLen = 20
	}

	// 构建表格
	var builder strings.Builder

	// 标题边框
	titleBorder := strings.Repeat("=", maxKeyLen+maxValueLen+7)
	builder.WriteString(titleBorder + "\n")
	builder.WriteString(fmt.Sprintf(" %s\n", title))
	builder.WriteString(titleBorder + "\n")

	// 表头
	headerBorder := fmt.Sprintf("+-%s-+-%s-+\n",
		strings.Repeat("-", maxKeyLen),
		strings.Repeat("-", maxValueLen),
	)
	builder.WriteString(headerBorder)
	builder.WriteString(fmt.Sprintf("| %-*s | %*s |\n",
		maxKeyLen, "项目",
		maxValueLen, "数值",
	))
	builder.WriteString(headerBorder)

	// 数据行（按key排序）
	keys := make([]string, 0, len(data))
	for k := range data {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		builder.WriteString(fmt.Sprintf("| %-*s | %*s |\n",
			maxKeyLen, k,
			maxValueLen, data[k],
		))
	}

	builder.WriteString(headerBorder)
	return builder.String()
}
