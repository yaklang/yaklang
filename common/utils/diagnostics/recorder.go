package diagnostics

import (
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

type Measurement struct {
	Name       string
	Total      time.Duration
	Count      uint64
	ErrorCount uint64
	Steps      []time.Duration
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
	measurement Measurement
}

func newMeasurementData(name string, stepCapacity int) *measurementData {
	steps := make([]time.Duration, stepCapacity)
	return &measurementData{
		measurement: Measurement{
			Name:       name,
			Steps:      steps,
			Total:      0,
			Count:      0,
			ErrorCount: 0,
		},
	}
}

func (m *measurementData) ensureStepCapacity(count int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if count <= len(m.measurement.Steps) {
		return
	}
	newSteps := make([]time.Duration, count)
	copy(newSteps, m.measurement.Steps)
	m.measurement.Steps = newSteps
}

func (m *measurementData) record(total time.Duration, stepDurations []time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(stepDurations) > len(m.measurement.Steps) {
		newSteps := make([]time.Duration, len(stepDurations))
		copy(newSteps, m.measurement.Steps)
		m.measurement.Steps = newSteps
	}
	for i, dur := range stepDurations {
		m.measurement.Steps[i] += dur
	}

	m.measurement.Total += total
	m.measurement.Count++
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
		Count:      m.measurement.Count,
		ErrorCount: m.measurement.ErrorCount,
		Steps:      steps,
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
	entry := r.entries.GetOrLoad(name, func() *measurementData {
		return newMeasurementData(name, stepCount)
	})
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
		switch {
		case a.Total < b.Total:
			return 1
		case a.Total > b.Total:
			return -1
		default:
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
	if r == nil || name == "" {
		return
	}
	entry, err := r.ensureEntry(name, 1)
	if err != nil || entry == nil {
		return
	}
	entry.record(duration, []time.Duration{duration})
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

// FormatPerformanceTable 格式化性能数据为表格
func FormatPerformanceTable(title string, measurements []Measurement) string {
	if len(measurements) == 0 {
		return fmt.Sprintf("No performance data for: %s", title)
	}

	// 计算列宽
	maxNameLen := len("Name")
	maxTimeLen := len("Duration")

	for _, m := range measurements {
		nameLen := len(m.Name)
		// 限制名称最大宽度为 80 字符
		if nameLen > 80 {
			nameLen = 80
		}
		if nameLen > maxNameLen {
			maxNameLen = nameLen
		}
		if len(m.Total.String()) > maxTimeLen {
			maxTimeLen = len(m.Total.String())
		}
	}

	// 确保最小宽度
	if maxNameLen < 30 {
		maxNameLen = 30
	}
	if maxTimeLen < 10 {
		maxTimeLen = 10
	}

	// 构建表格
	var builder strings.Builder

	// 标题边框
	titleBorder := strings.Repeat("=", maxNameLen+maxTimeLen+7)
	builder.WriteString(titleBorder + "\n")
	builder.WriteString(fmt.Sprintf(" %s\n", title))
	builder.WriteString(titleBorder + "\n")

	// 表头
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

	// 数据行
	for _, m := range measurements {
		// 截断过长的名称
		displayName := m.Name
		if len(displayName) > 80 {
			displayName = displayName[:77] + "..."
		}
		builder.WriteString(fmt.Sprintf("| %-*s | %*s |\n",
			maxNameLen, displayName,
			maxTimeLen, m.Total.String(),
		))
	}

	builder.WriteString(headerBorder)
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
