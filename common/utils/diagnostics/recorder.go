package diagnostics

import (
	"errors"
	"fmt"
	"io"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

// Recorder collects measurements and optional nested TRACE output.
type Recorder struct {
	entries *utils.SafeMap[*Measurement]

	stepsMu sync.Mutex
	steps   []Step

	runMu           sync.Mutex
	nested          bool
	nestedLog       bool
	nestedMin       time.Duration
	nestedWriter    io.Writer
	runDepth        int
	lastLoggedDepth int32
	traceStates     sync.Map
}

func NewRecorder() *Recorder {
	return &Recorder{
		entries:         utils.NewSafeMap[*Measurement](),
		nestedWriter:    logLineWriter{},
		lastLoggedDepth: -1,
	}
}

func (r *Recorder) TraceLab(lab Lab, fn func() error) error {
	return r.trace(lab, fn, true)
}

func (r *Recorder) Track(name string, steps ...func() error) error {
	return r.trackLevel(LevelNormal, name, steps...)
}

func (r *Recorder) TrackLow(name string, steps ...func() error) error {
	return r.trackLevel(LevelLow, name, steps...)
}

func (r *Recorder) TrackHigh(name string, steps ...func() error) error {
	return r.trackLevel(LevelHigh, name, steps...)
}

func (r *Recorder) SetNested(enabled bool) {
	if r == nil {
		return
	}
	r.runMu.Lock()
	r.nested = enabled
	r.runMu.Unlock()
}

func (r *Recorder) SetNestedLog(log bool, minDuration time.Duration, w io.Writer) {
	if r == nil {
		return
	}
	r.runMu.Lock()
	r.nestedLog = log
	r.nestedMin = minDuration
	if w != nil {
		r.nestedWriter = w
	} else {
		r.nestedWriter = logLineWriter{}
	}
	r.runMu.Unlock()
}

func (r *Recorder) NestedEnabled() bool {
	if r == nil {
		return false
	}
	r.runMu.Lock()
	defer r.runMu.Unlock()
	return r.nested
}

func (r *Recorder) Steps() []Step {
	if r == nil {
		return nil
	}
	r.stepsMu.Lock()
	defer r.stepsMu.Unlock()
	out := make([]Step, len(r.steps))
	copy(out, r.steps)
	return out
}

func (r *Recorder) ensureMeasurement(name string) (*Measurement, error) {
	if name == "" {
		return nil, errors.New("diagnostics: measurement name is empty")
	}
	if r == nil {
		return nil, nil
	}
	return r.entries.GetOrLoad(name, func() (*Measurement, error) {
		return newMeasurement(name), nil
	})
}

func (r *Recorder) trackLevel(lvl Level, name string, steps ...func() error) error {
	return r.track(Enabled(lvl), name, steps...)
}

func (r *Recorder) track(enabled bool, name string, steps ...func() error) error {
	if name == "" {
		return errors.New("diagnostics: measurement name is empty")
	}
	if !enabled || r == nil {
		return runStepsWithoutRecording(steps)
	}
	n := len(steps)
	for i, step := range steps {
		if step == nil {
			continue
		}
		if err := r.trace(TrackStepLab(name, i, n), step, false); err != nil {
			return err
		}
	}
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
	for _, m := range values {
		if m != nil {
			result = append(result, m.snapshot())
		}
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
	r.entries = utils.NewSafeMap[*Measurement]()
	r.stepsMu.Lock()
	r.steps = nil
	r.stepsMu.Unlock()
	r.runMu.Lock()
	r.runDepth = 0
	r.lastLoggedDepth = -1
	r.traceStates = sync.Map{}
	r.runMu.Unlock()
}

func (r *Recorder) RecordDuration(name string, duration time.Duration) {
	if r == nil || name == "" {
		return
	}
	end := time.Now()
	r.finishTrace(NewLab(LabName(name)), end.Add(-duration), 0, nil)
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
		return
	}
	snapshots := rec.Snapshot()
	if len(snapshots) == 0 {
		log.Infof("recorder is empty")
		return
	}
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
	dbSnap := database.Snapshot()
	memIndex := make(map[string]Measurement, len(memory.Snapshot()))
	for _, s := range memory.Snapshot() {
		memIndex[s.Name] = s
	}
	for _, dbm := range dbSnap {
		mem, ok := memIndex[dbm.Name]
		if !ok {
			log.Debugf("Measurement [%s] not found in memory cost", dbm.Name)
			continue
		}
		if mem.Count == 0 {
			mem.Count = 1
		}
		if dbm.Count > mem.Count*5 || dbm.Total > mem.Total*2 {
			log.Debugf("Measurement [%s] mismatch: db %v mem %v", dbm.Name, dbm.Total, mem.Total)
		}
	}
}

func FormatPerformanceTable(title string, measurements []Measurement) string {
	if len(measurements) == 0 {
		return fmt.Sprintf("No performance data for: %s", title)
	}
	maxName, maxTime := len("Name"), len("Duration")
	for _, m := range measurements {
		nl := len(m.Name)
		if nl > 80 {
			nl = 80
		}
		if nl > maxName {
			maxName = nl
		}
		if tl := len(m.Total.String()); tl > maxTime {
			maxTime = tl
		}
	}
	if maxName < 30 {
		maxName = 30
	}
	if maxTime < 10 {
		maxTime = 10
	}
	var b strings.Builder
	border := strings.Repeat("=", maxName+maxTime+7)
	b.WriteString(border + "\n " + title + "\n" + border + "\n")
	hdr := fmt.Sprintf("+-%s-+-%s-+\n", strings.Repeat("-", maxName), strings.Repeat("-", maxTime))
	b.WriteString(hdr)
	b.WriteString(fmt.Sprintf("| %-*s | %*s |\n", maxName, "Name", maxTime, "Duration"))
	b.WriteString(hdr)
	for _, m := range measurements {
		name := m.Name
		if len(name) > 80 {
			name = name[:77] + "..."
		}
		b.WriteString(fmt.Sprintf("| %-*s | %*s |\n", maxName, name, maxTime, m.Total.String()))
	}
	b.WriteString(hdr)
	return b.String()
}

func FormatSimpleTable(title string, data map[string]string) string {
	if len(data) == 0 {
		return fmt.Sprintf("No data for: %s", title)
	}
	maxKey, maxVal := len("Key"), len("Value")
	for k, v := range data {
		if len(k) > maxKey {
			maxKey = len(k)
		}
		if len(v) > maxVal {
			maxVal = len(v)
		}
	}
	if maxKey < 20 {
		maxKey = 20
	}
	if maxVal < 20 {
		maxVal = 20
	}
	var b strings.Builder
	border := strings.Repeat("=", maxKey+maxVal+7)
	b.WriteString(border + "\n " + title + "\n" + border + "\n")
	hdr := fmt.Sprintf("+-%s-+-%s-+\n", strings.Repeat("-", maxKey), strings.Repeat("-", maxVal))
	b.WriteString(hdr)
	b.WriteString(fmt.Sprintf("| %-*s | %*s |\n", maxKey, "项目", maxVal, "数值"))
	b.WriteString(hdr)
	keys := make([]string, 0, len(data))
	for k := range data {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		b.WriteString(fmt.Sprintf("| %-*s | %*s |\n", maxKey, k, maxVal, data[k]))
	}
	b.WriteString(hdr)
	return b.String()
}
