package diagnostics

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

type StepFunc func() error

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

	for index, t := range m.Steps {
		stepAvg := time.Duration(0)
		if m.Count > 0 {
			stepAvg = t / time.Duration(m.Count)
		}
		builder.WriteString(fmt.Sprintf("%s-%-4d\tTime: %v\tCount: %v\tAvg: %v\n",
			m.Name, index+1, t, m.Count, stepAvg,
		))
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

func (r *Recorder) Track(enabled bool, name string, steps ...func()) {
	stepFuncs := make([]StepFunc, 0, len(steps))
	for _, step := range steps {
		if step == nil {
			continue
		}
		fn := step
		stepFuncs = append(stepFuncs, func() error {
			fn()
			return nil
		})
	}
	if _, err := r.TrackWithError(enabled, name, stepFuncs...); err != nil {
		log.Debugf("diagnostics: track %s error: %v", name, err)
	}
}

func (r *Recorder) TrackWithError(enabled bool, name string, steps ...StepFunc) (Measurement, error) {
	empty := Measurement{Name: name}
	if name == "" {
		return empty, errors.New("diagnostics: measurement name is empty")
	}

	if !enabled || r == nil {
		for _, step := range steps {
			if step == nil {
				continue
			}
			if err := step(); err != nil {
				return empty, err
			}
		}
		return empty, nil
	}

	entry, err := r.ensureEntry(name, len(steps))
	if err != nil {
		return empty, err
	}
	if entry == nil {
		return empty, nil
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
			return empty, err
		}
		elapsed := time.Since(start)
		durations[i] = elapsed
		total += elapsed
	}

	entry.record(total, durations)
	return entry.snapshot(), nil
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

var defaultRecorder struct {
	mu       sync.RWMutex
	recorder *Recorder
}

func init() {
	defaultRecorder.recorder = NewRecorder()
}

func DefaultRecorder() *Recorder {
	defaultRecorder.mu.RLock()
	defer defaultRecorder.mu.RUnlock()
	return defaultRecorder.recorder
}

func ReplaceDefault(rec *Recorder) *Recorder {
	if rec == nil {
		rec = NewRecorder()
	}
	defaultRecorder.mu.Lock()
	old := defaultRecorder.recorder
	defaultRecorder.recorder = rec
	defaultRecorder.mu.Unlock()
	return old
}

func ResetDefaultRecorder() *Recorder {
	rec := NewRecorder()
	ReplaceDefault(rec)
	return rec
}

func Track(enabled bool, name string, steps ...func()) {
	DefaultRecorder().Track(enabled, name, steps...)
}

func TrackWithError(enabled bool, name string, steps ...StepFunc) (Measurement, error) {
	return DefaultRecorder().TrackWithError(enabled, name, steps...)
}

func LogCompileSummary() {
	LogRecorder("compile", DefaultRecorder())
}

func LogRecorder(label string, recorders ...*Recorder) {
	if len(recorders) == 0 {
		recorders = []*Recorder{DefaultRecorder()}
	}
	for _, rec := range recorders {
		logRecorder(label, rec)
	}
}

func logRecorder(label string, rec *Recorder) {
	if rec == nil {
		log.Infof("recorder %s is nil", label)
		return
	}
	snapshots := rec.Snapshot()
	if len(snapshots) == 0 {
		log.Infof("recorder %s is empty", label)
		return
	}
	log.Infof("========================================")
	log.Infof("Measurement Summary [%s]", label)
	log.Infof("========================================")
	for _, snapshot := range snapshots {
		log.Infof(snapshot.String())
	}
	log.Infof("========================================")
}

func LogScanPerformance(ruleRecorder *Recorder, enableRulePerformance bool, totalDuration time.Duration) {
	LogCompileSummary()

	totalCount := uint64(0)
	if ruleRecorder != nil {
		for _, measurement := range ruleRecorder.Snapshot() {
			totalCount += measurement.Count
		}
	}
	if totalCount == 0 {
		totalCount = 1
	}

	avgDuration := totalDuration / time.Duration(totalCount)
	log.Infof("=== Scan Total ===")
	log.Infof("Time: %v\tCount: %d\tAvg: %v", totalDuration, totalCount, avgDuration)
	log.Infof("==================")

	if !enableRulePerformance || ruleRecorder == nil {
		return
	}

	snapshots := ruleRecorder.Snapshot()
	if len(snapshots) == 0 {
		return
	}

	log.Infof("=== Rule Performance (scan) ===")
	for _, measurement := range snapshots {
		if measurement.Count == 0 {
			continue
		}
		avg := measurement.Total / time.Duration(measurement.Count)
		log.Infof("%s Time: %v Count: %d Avg: %v", measurement.Name, measurement.Total, measurement.Count, avg)
	}
	log.Infof("================================")
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
