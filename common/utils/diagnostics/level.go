package diagnostics

import (
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/yaklang/yaklang/common/log"
)

const envDiagnosticsLevel = "YAK_DIAGNOSTICS_LOG_LEVEL"

// Level controls the detail tier used when recording diagnostics measurements.
type Level int

const (
	LevelTrace Level = iota
	LevelMeasure
	LevelFocus
	LevelCritical
	LevelOff
)

var levelNames = map[string]Level{
	"trace":    LevelTrace,
	"detail":   LevelTrace,
	"verbose":  LevelTrace,
	"measure":  LevelMeasure,
	"monitor":  LevelMeasure,
	"routine":  LevelMeasure,
	"focus":    LevelFocus,
	"high":     LevelFocus,
	"alert":    LevelFocus,
	"critical": LevelCritical,
	"signal":   LevelCritical,
	"off":      LevelOff,
}

var levelStrings = map[Level]string{
	LevelTrace:    "trace",
	LevelMeasure:  "measure",
	LevelFocus:    "focus",
	LevelCritical: "critical",
	LevelOff:      "off",
}

var (
	levelMu sync.RWMutex
	level   = LevelMeasure
)

func init() {
	if raw := strings.TrimSpace(os.Getenv(envDiagnosticsLevel)); raw != "" {
		if err := SetLevelFromString(raw); err != nil {
			log.Warnf("diagnostics: ignoring invalid log level %q: %v", raw, err)
		}
	}
}

// SetLevel overrides the diagnostics log level manually.
func SetLevel(lvl Level) {
	levelMu.Lock()
	level = lvl
	levelMu.Unlock()
}

// SetLevelFromString parses a string and applies the log level if valid.
func SetLevelFromString(raw string) error {
	parsed, ok := parseLevel(raw)
	if !ok {
		return fmt.Errorf("unknown diagnostics log level: %s", raw)
	}
	SetLevel(parsed)
	return nil
}

// GetLevel returns the current diagnostics log level.
func GetLevel() Level {
	levelMu.RLock()
	defer levelMu.RUnlock()
	return level
}

func parseLevel(raw string) (Level, bool) {
	normalized := strings.ToLower(strings.TrimSpace(raw))
	lvl, ok := levelNames[normalized]
	return lvl, ok
}

func (lvl Level) String() string {
	if s, ok := levelStrings[lvl]; ok {
		return s
	}
	return fmt.Sprintf("level-%d", lvl)
}

// Enabled determines if messages at the requested level should be emitted.
func Enabled(lvl Level) bool {
	if lvl == LevelOff {
		return false
	}
	return lvl >= GetLevel()
}

// API
func (r *Recorder) TrackLevel(lvl Level, name string, steps ...StepFunc) error {
	return r.Track(Enabled(lvl), name, steps...)
}

func TrackLevel(lvl Level, name string, steps ...StepFunc) error {
	return DefaultRecorder().TrackLevel(lvl, name, steps...)
}

func (r *Recorder) TrackTrace(name string, steps ...StepFunc) error {
	return r.TrackLevel(LevelTrace, name, steps...)
}

func TrackTrace(name string, steps ...StepFunc) error {
	return DefaultRecorder().TrackTrace(name, steps...)
}

func (r *Recorder) TrackMeasure(name string, steps ...StepFunc) error {
	return r.TrackLevel(LevelMeasure, name, steps...)
}

func TrackMeasure(name string, steps ...StepFunc) error {
	return DefaultRecorder().TrackMeasure(name, steps...)
}

func (r *Recorder) TrackFocus(name string, steps ...StepFunc) error {
	return r.TrackLevel(LevelFocus, name, steps...)
}

func TrackFocus(name string, steps ...StepFunc) error {
	return DefaultRecorder().TrackFocus(name, steps...)
}

func (r *Recorder) TrackCritical(name string, steps ...StepFunc) error {
	return r.TrackLevel(LevelCritical, name, steps...)
}

func TrackCritical(name string, steps ...StepFunc) error {
	return DefaultRecorder().TrackCritical(name, steps...)
}
