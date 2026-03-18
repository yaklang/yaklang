package diagnostics

import (
	"time"
)

// Track API：按 level 控制是否记录，Level 判断在内部处理
//
//	TrackLow  - LevelLow 时记录
//	Track     - LevelNormal 时记录
//	TrackHigh - LevelHigh 时记录，任意非 off 级别均记录

// KindRecorder 方法：不含 kind 参数，使用 ForKind 时绑定的 kind；均返回 (duration, error)
func (k *KindRecorder) Track(name string, steps ...func() error) (time.Duration, error) {
	if k == nil || k.rec == nil {
		return 0, runSteps(steps)
	}
	return k.rec.trackWithDuration(Enabled(LevelNormal), true, k.kind, name, -1, steps...)
}

func (k *KindRecorder) TrackLow(name string, steps ...func() error) (time.Duration, error) {
	if k == nil || k.rec == nil {
		return 0, runSteps(steps)
	}
	return k.rec.trackWithDuration(Enabled(LevelLow), true, k.kind, name, -1, steps...)
}

func (k *KindRecorder) TrackHigh(name string, steps ...func() error) (time.Duration, error) {
	if k == nil || k.rec == nil {
		return 0, runSteps(steps)
	}
	return k.rec.trackWithDuration(Enabled(LevelHigh), true, k.kind, name, -1, steps...)
}

func (k *KindRecorder) TrackLowLog(name string, steps ...func() error) error {
	if k == nil || k.rec == nil {
		return runSteps(steps)
	}
	_, err := k.rec.trackWithDuration(Enabled(LevelHigh), true, k.kind, name, -1, steps...)
	return err
}

func runSteps(steps []func() error) error {
	for _, step := range steps {
		if step != nil {
			if err := step(); err != nil {
				return err
			}
		}
	}
	return nil
}

// --- Recorder 原 Track 方法（含 kind 参数，保留兼容）；均返回 (duration, error) ---

// TrackLow 按 LevelLow 记录；输出 LogLow
func (r *Recorder) TrackLow(kind TrackKind, name string, steps ...func() error) (time.Duration, error) {
	return r.trackWithDuration(Enabled(LevelLow), true, kind, name, -1, steps...)
}

// Track 按 LevelNormal 记录；LevelHigh 时也记录；输出 LogLow
func (r *Recorder) Track(kind TrackKind, name string, steps ...func() error) (time.Duration, error) {
	return r.trackWithDuration(Enabled(LevelNormal), true, kind, name, -1, steps...)
}

// TrackHigh 按 LevelHigh 记录；任意非 off 级别均记录；输出 LogLow
func (r *Recorder) TrackHigh(kind TrackKind, name string, steps ...func() error) (time.Duration, error) {
	return r.trackWithDuration(Enabled(LevelHigh), true, kind, name, -1, steps...)
}

// Trace 单步测量的推荐入口：按 LevelHigh 记录并输出 LogLow
func (r *Recorder) Trace(kind TrackKind, name string, fn func() error) (time.Duration, error) {
	return r.trackWithDuration(Enabled(LevelHigh), true, kind, name, -1, fn)
}

// TrackLowLog 记录 when LevelHigh，输出 LogLow；用于 database save 等总耗时场景
func (r *Recorder) TrackLowLog(kind TrackKind, name string, steps ...func() error) error {
	_, err := r.trackWithDuration(Enabled(LevelHigh), true, kind, name, -1, steps...)
	return err
}

// TrackLow/Track/TrackHigh 包级便捷入口，使用 DefaultRecorder
func TrackLow(kind TrackKind, name string, steps ...func() error) (time.Duration, error) {
	return DefaultRecorder().TrackLow(kind, name, steps...)
}

func Track(kind TrackKind, name string, steps ...func() error) (time.Duration, error) {
	return DefaultRecorder().Track(kind, name, steps...)
}

func TrackHigh(kind TrackKind, name string, steps ...func() error) (time.Duration, error) {
	return DefaultRecorder().TrackHigh(kind, name, steps...)
}

// Trace 单步测量，使用 DefaultRecorder
func Trace(kind TrackKind, name string, fn func() error) (time.Duration, error) {
	return DefaultRecorder().Trace(kind, name, fn)
}

// RunStepsWithTrack 简便入口：rec 为 nil 时使用 DefaultRecorder；仅当 env 关闭性能日志时不记录
func RunStepsWithTrack(rec *Recorder, name string, steps ...func() error) error {
	if rec == nil {
		rec = DefaultRecorder()
	}
	_, err := rec.ForKind(TrackKindGeneral).Track(name, steps...)
	return err
}

// TrackWithParams 供 SSA 等调用方定制 enabled/doLog/kind/name/depth 的底层入口
func (r *Recorder) TrackWithParams(enabled bool, doLog bool, kind TrackKind, name string, depth int, steps ...func() error) (time.Duration, error) {
	if r == nil {
		for _, step := range steps {
			if step != nil {
				if err := step(); err != nil {
					return 0, err
				}
			}
		}
		return 0, nil
	}
	return r.trackWithDuration(enabled, doLog, kind, name, depth, steps...)
}
