// automatically generated by stateify.

package sleep

import (
	"context"

	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/state"
)

func (s *Sleeper) StateTypeName() string {
	return "pkg/sleep.Sleeper"
}

func (s *Sleeper) StateFields() []string {
	return []string{
		"sharedList",
		"localList",
		"allWakers",
	}
}

func (s *Sleeper) beforeSave() {}

// +checklocksignore
func (s *Sleeper) StateSave(stateSinkObject state.Sink) {
	s.beforeSave()
	var sharedListValue *Waker
	sharedListValue = s.saveSharedList()
	stateSinkObject.SaveValue(0, sharedListValue)
	stateSinkObject.Save(1, &s.localList)
	stateSinkObject.Save(2, &s.allWakers)
}

func (s *Sleeper) afterLoad(context.Context) {}

// +checklocksignore
func (s *Sleeper) StateLoad(ctx context.Context, stateSourceObject state.Source) {
	stateSourceObject.Load(1, &s.localList)
	stateSourceObject.Load(2, &s.allWakers)
	stateSourceObject.LoadValue(0, new(*Waker), func(y any) { s.loadSharedList(ctx, y.(*Waker)) })
}

func (w *Waker) StateTypeName() string {
	return "pkg/sleep.Waker"
}

func (w *Waker) StateFields() []string {
	return []string{
		"s",
		"next",
		"allWakersNext",
	}
}

func (w *Waker) beforeSave() {}

// +checklocksignore
func (w *Waker) StateSave(stateSinkObject state.Sink) {
	w.beforeSave()
	var sValue wakerState
	sValue = w.saveS()
	stateSinkObject.SaveValue(0, sValue)
	stateSinkObject.Save(1, &w.next)
	stateSinkObject.Save(2, &w.allWakersNext)
}

func (w *Waker) afterLoad(context.Context) {}

// +checklocksignore
func (w *Waker) StateLoad(ctx context.Context, stateSourceObject state.Source) {
	stateSourceObject.Load(1, &w.next)
	stateSourceObject.Load(2, &w.allWakersNext)
	stateSourceObject.LoadValue(0, new(wakerState), func(y any) { w.loadS(ctx, y.(wakerState)) })
}

func (w *wakerState) StateTypeName() string {
	return "pkg/sleep.wakerState"
}

func (w *wakerState) StateFields() []string {
	return []string{
		"asserted",
		"other",
	}
}

func (w *wakerState) beforeSave() {}

// +checklocksignore
func (w *wakerState) StateSave(stateSinkObject state.Sink) {
	w.beforeSave()
	stateSinkObject.Save(0, &w.asserted)
	stateSinkObject.Save(1, &w.other)
}

func (w *wakerState) afterLoad(context.Context) {}

// +checklocksignore
func (w *wakerState) StateLoad(ctx context.Context, stateSourceObject state.Source) {
	stateSourceObject.Load(0, &w.asserted)
	stateSourceObject.Load(1, &w.other)
}

func init() {
	state.Register((*Sleeper)(nil))
	state.Register((*Waker)(nil))
	state.Register((*wakerState)(nil))
}
