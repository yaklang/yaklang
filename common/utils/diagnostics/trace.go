package diagnostics

import (
	"fmt"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/yaklang/yaklang/common/utils"
)

const (
	traceHeader     = "TRACE"
	traceIndentUnit = "    "
	traceBeginMark  = `\`
	traceEndMark    = `/`
	traceLeafMark   = `|`
)

// Step is one execution record.
type Step struct {
	Name      string
	Kind      string
	Text      string
	Desc      string
	StepIndex int
	Start     time.Time
	End       time.Time
	Duration  time.Duration
	Err       string
}

type traceSpanFrame struct {
	depth         int
	lab           Lab
	beginDeferred bool
	hadChild      bool
	goroutineID   int64
	asyncWorker   bool
}

type goroutineTraceState struct {
	spanStack []traceSpanFrame
	runDepth  int
}

type traceLineMeta struct {
	goroutineID int64
	asyncWorker bool
}

func stepFromLab(lab Lab, start, end time.Time, err error) Step {
	s := Step{
		Name:      lab.Name,
		Kind:      lab.Kind,
		Text:      lab.Display(),
		Desc:      lab.Desc,
		StepIndex: lab.StepIndex,
		Start:     start,
		End:       end,
		Duration:  end.Sub(start),
	}
	if err != nil {
		s.Err = err.Error()
	}
	return s
}

func (s Step) displayText() string {
	if t := strings.TrimSpace(s.Text); t != "" {
		return t
	}
	if n := strings.TrimSpace(s.Name); n != "" {
		return n
	}
	return "run"
}

func currentGoroutineID() int64 {
	buf := make([]byte, 64)
	n := runtime.Stack(buf, false)
	if n <= 0 {
		return 0
	}
	fields := strings.Fields(string(buf[:n]))
	if len(fields) < 2 {
		return 0
	}
	id, err := strconv.ParseInt(fields[1], 10, 64)
	if err != nil {
		return 0
	}
	return id
}

func formatGoroutineID(id int64) string {
	return strconv.FormatInt(id, 10)
}

func formatTraceLine(depth int, mark, name, dur, desc, err string, meta traceLineMeta) string {
	var b strings.Builder
	b.WriteString(traceHeader)
	if depth > 0 {
		b.WriteString(strings.Repeat(traceIndentUnit, depth))
	}
	b.WriteString(" ")
	b.WriteString(mark)
	b.WriteString(" ")
	b.WriteString(strings.TrimSpace(name))
	if meta.asyncWorker && meta.goroutineID > 0 {
		b.WriteString("  g=")
		b.WriteString(formatGoroutineID(meta.goroutineID))
	}
	if dur != "" {
		b.WriteString("  dur=" + dur)
	}
	if d := strings.TrimSpace(desc); d != "" {
		b.WriteString("  " + d)
	}
	if err != "" {
		b.WriteString(`  err="` + strings.ReplaceAll(err, `"`, `'`) + `"`)
	}
	return b.String()
}

// trace runs fn, records Step/Measurement, and prints nested TRACE lines.
// nestDepth=true for TraceLab (lazybuild nesting); false for Track sub-steps.
func (r *Recorder) trace(lab Lab, fn func() error, nestDepth bool) (err error) {
	if r == nil {
		if fn != nil {
			return fn()
		}
		return nil
	}
	if fn == nil {
		return nil
	}
	if lab.Key() == "" {
		return fmt.Errorf("diagnostics: lab key is empty")
	}

	start := time.Now()
	gid := currentGoroutineID()
	var depth int
	var asyncWorker bool
	if r.traceLiveEnabled() {
		depth, asyncWorker = r.traceLogDepthInfo()
		if nestDepth {
			r.pushRunDepth()
			defer r.popRunDepth()
		}
	} else if nestDepth {
		depth = r.pushRunDepth()
		defer r.popRunDepth()
	}
	r.pushTraceSpan(depth, lab, gid, asyncWorker)
	r.emitTraceBegin(depth, lab)

	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("panic: %v", recovered)
			utils.PrintCurrentGoroutineRuntimeStack()
		}
		r.finishTrace(lab, start, depth, err)
	}()

	return fn()
}

func (r *Recorder) traceLiveEnabled() bool {
	if r == nil {
		return false
	}
	r.runMu.Lock()
	defer r.runMu.Unlock()
	return r.nestedLog && r.nested
}

func (r *Recorder) traceLogDepthInfo() (depth int, asyncWorker bool) {
	if r == nil {
		return 0, false
	}
	r.runMu.Lock()
	defer r.runMu.Unlock()
	gid := currentGoroutineID()
	state := r.currentTraceStateLocked(gid)
	if local := len(state.spanStack); local > 0 {
		return local, false
	}
	inherited := r.activeSpanDepthElsewhere(gid)
	return inherited, inherited > 0
}

func (r *Recorder) currentTraceStateLocked(gid int64) *goroutineTraceState {
	if state, ok := r.traceStates.Load(gid); ok {
		return state.(*goroutineTraceState)
	}
	state := &goroutineTraceState{}
	r.traceStates.Store(gid, state)
	return state
}

func (r *Recorder) maybeDeleteTraceState(gid int64, state *goroutineTraceState) {
	if r == nil || state == nil {
		return
	}
	if len(state.spanStack) == 0 && state.runDepth == 0 {
		r.traceStates.Delete(gid)
	}
}

func (r *Recorder) activeSpanDepthElsewhere(self int64) int {
	maxDepth := 0
	r.traceStates.Range(func(key, value any) bool {
		if key.(int64) == self {
			return true
		}
		if d := len(value.(*goroutineTraceState).spanStack); d > maxDepth {
			maxDepth = d
		}
		return true
	})
	return maxDepth
}

func (r *Recorder) pushTraceSpan(depth int, lab Lab, gid int64, asyncWorker bool) {
	if r == nil {
		return
	}
	r.runMu.Lock()
	defer r.runMu.Unlock()
	if !r.nestedLog || !r.nested {
		return
	}
	state := r.currentTraceStateLocked(gid)
	state.spanStack = append(state.spanStack, traceSpanFrame{
		depth: depth, lab: lab, beginDeferred: true,
		goroutineID: gid, asyncWorker: asyncWorker,
	})
}

func (r *Recorder) popTraceSpan() (traceSpanFrame, bool) {
	if r == nil {
		return traceSpanFrame{}, false
	}
	r.runMu.Lock()
	defer r.runMu.Unlock()
	gid := currentGoroutineID()
	state := r.currentTraceStateLocked(gid)
	n := len(state.spanStack)
	if n == 0 {
		return traceSpanFrame{}, false
	}
	frame := state.spanStack[n-1]
	state.spanStack = state.spanStack[:n-1]
	r.maybeDeleteTraceState(gid, state)
	return frame, true
}

func (r *Recorder) spanStackLocked() []traceSpanFrame {
	return r.currentTraceStateLocked(currentGoroutineID()).spanStack
}

func (r *Recorder) noteTraceParentActivity(childDepth int) {
	r.noteTraceOutputActivity(childDepth)
}

func (r *Recorder) noteTraceAncestorActivity(endedDepth int) {
	r.noteTraceOutputActivity(endedDepth)
}

func (r *Recorder) noteTraceOutputActivity(depth int) {
	if r == nil {
		return
	}
	r.runMu.Lock()
	defer r.runMu.Unlock()
	r.traceStates.Range(func(_key, value any) bool {
		state := value.(*goroutineTraceState)
		for i := range state.spanStack {
			if state.spanStack[i].depth < depth {
				r.markTraceSpanFrameHadChild(state, i)
			}
		}
		return true
	})
}

func (r *Recorder) markTraceSpanFrameHadChild(state *goroutineTraceState, i int) {
	if state == nil || i < 0 || i >= len(state.spanStack) {
		return
	}
	f := &state.spanStack[i]
	if f.hadChild {
		return
	}
	f.hadChild = true
	if f.beginDeferred {
		r.emitTraceBeginUnlocked(f.depth, f.lab, traceLineMeta{goroutineID: f.goroutineID, asyncWorker: f.asyncWorker})
		f.beginDeferred = false
	}
}

func (r *Recorder) traceBeginDeferred(depth int, lab Lab) bool {
	if r == nil {
		return false
	}
	r.runMu.Lock()
	defer r.runMu.Unlock()
	if !r.nestedLog || !r.nested {
		return false
	}
	stack := r.spanStackLocked()
	if n := len(stack); n > 0 {
		top := &stack[n-1]
		if top.depth == depth && top.lab.Key() == lab.Key() {
			return top.beginDeferred
		}
	}
	return false
}

func (r *Recorder) pushRunDepth() int {
	if r == nil || !r.NestedEnabled() {
		return 0
	}
	r.runMu.Lock()
	defer r.runMu.Unlock()
	if r.nestedLog && r.nested {
		gid := currentGoroutineID()
		state := r.currentTraceStateLocked(gid)
		depth := state.runDepth
		state.runDepth++
		return depth
	}
	depth := r.runDepth
	r.runDepth++
	return depth
}

func (r *Recorder) popRunDepth() {
	if r == nil || !r.NestedEnabled() {
		return
	}
	r.runMu.Lock()
	defer r.runMu.Unlock()
	if r.nestedLog && r.nested {
		gid := currentGoroutineID()
		state := r.currentTraceStateLocked(gid)
		if state.runDepth > 0 {
			state.runDepth--
		}
		r.maybeDeleteTraceState(gid, state)
		return
	}
	if r.runDepth > 0 {
		r.runDepth--
	}
}

func (r *Recorder) appendStep(lab Lab, start, end time.Time, err error) Step {
	step := stepFromLab(lab, start, end, err)
	r.stepsMu.Lock()
	r.steps = append(r.steps, step)
	r.stepsMu.Unlock()
	return step
}

func (r *Recorder) finishTrace(lab Lab, start time.Time, depth int, err error) {
	if r == nil {
		return
	}
	step := r.appendStep(lab, start, time.Now(), err)
	if m, mErr := r.ensureMeasurement(lab.Key()); mErr == nil && m != nil {
		if err != nil {
			m.markError()
		} else {
			m.absorb(step.Duration, lab.StepIndex)
		}
	}
	frame, hasFrame := r.popTraceSpan()
	collapsed := hasFrame && frame.beginDeferred && !frame.hadChild
	meta := traceLineMeta{}
	if hasFrame {
		meta = traceLineMeta{goroutineID: frame.goroutineID, asyncWorker: frame.asyncWorker}
	}
	r.emitTraceEnd(depth, step, err, collapsed, meta)
}

func (r *Recorder) emitTraceBegin(depth int, lab Lab) {
	if r == nil {
		return
	}
	if r.traceBeginDeferred(depth, lab) {
		return
	}
	r.noteTraceParentActivity(depth)
	r.emitTraceBeginLocked(depth, lab, r.currentSpanLineMeta())
}

func (r *Recorder) currentSpanLineMeta() traceLineMeta {
	if r == nil {
		return traceLineMeta{}
	}
	r.runMu.Lock()
	defer r.runMu.Unlock()
	stack := r.spanStackLocked()
	if len(stack) == 0 {
		return traceLineMeta{}
	}
	top := stack[len(stack)-1]
	return traceLineMeta{goroutineID: top.goroutineID, asyncWorker: top.asyncWorker}
}

func (r *Recorder) emitTraceBeginLocked(depth int, lab Lab, meta traceLineMeta) {
	if r == nil {
		return
	}
	r.runMu.Lock()
	defer r.runMu.Unlock()
	r.emitTraceBeginUnlocked(depth, lab, meta)
}

func (r *Recorder) emitTraceBeginUnlocked(depth int, lab Lab, meta traceLineMeta) {
	if r == nil || !r.nestedLog {
		return
	}
	w := r.nestedWriter
	if w == nil {
		w = logLineWriter{}
	}
	line := formatTraceLine(depth, traceBeginMark, lab.Display(), "", lab.Desc, "", meta) + "\n"
	fmt.Fprint(w, line)
}

func (r *Recorder) emitTraceEnd(depth int, step Step, err error, collapsed bool, meta traceLineMeta) {
	if r == nil {
		return
	}
	r.runMu.Lock()
	nestedLog := r.nestedLog
	min := r.nestedMin
	w := r.nestedWriter
	r.runMu.Unlock()

	if !nestedLog || !r.shouldLogStep(depth, step.Duration, err, min) {
		return
	}
	if w == nil {
		w = logLineWriter{}
	}
	r.noteTraceAncestorActivity(depth)
	dur := ""
	if step.Duration > 0 {
		dur = step.Duration.String()
	}
	mark := traceEndMark
	if collapsed {
		mark = traceLeafMark
	}
	line := formatTraceLine(depth, mark, step.displayText(), dur, step.Desc, step.Err, meta) + "\n"
	fmt.Fprint(w, line)
	atomic.StoreInt32(&r.lastLoggedDepth, int32(depth))
}

func (r *Recorder) shouldLogStep(depth int, duration time.Duration, err error, min time.Duration) bool {
	if err != nil {
		return true
	}
	if duration >= min {
		return true
	}
	last := atomic.LoadInt32(&r.lastLoggedDepth)
	if last >= 0 && int32(depth) < last {
		atomic.StoreInt32(&r.lastLoggedDepth, int32(depth))
		return true
	}
	return false
}
