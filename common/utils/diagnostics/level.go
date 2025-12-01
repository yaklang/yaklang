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
	LevelLow    Level = iota
	LevelNormal       // default
	LevelHigh
	LevelOff
)

var levelNames = map[string]Level{
	"trace":    LevelLow,
	"detail":   LevelLow,
	"verbose":  LevelLow,
	"measure":  LevelNormal,
	"monitor":  LevelNormal,
	"routine":  LevelNormal,
	"critical": LevelHigh,
	"signal":   LevelHigh,
	"off":      LevelOff,
}

var levelStrings = map[Level]string{
	LevelLow:    "trace",
	LevelNormal: "measure",
	LevelHigh:   "critical",
	LevelOff:    "off",
}

var (
	levelMu sync.RWMutex
	level   = LevelNormal
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
func (r *Recorder) trackLevel(lvl Level, name string, steps ...func() error) error {
	return r.track(Enabled(lvl), name, steps...)
}

func (r *Recorder) TrackLow(name string, steps ...func() error) error {
	return r.trackLevel(LevelLow, name, steps...)
}

func TrackLow(name string, steps ...func() error) error {
	return DefaultRecorder().TrackLow(name, steps...)
}

func Track(name string, steps ...func() error) error {
	return DefaultRecorder().Track(name, steps...)
}

func (r *Recorder) Track(name string, steps ...func() error) error {
	return r.trackLevel(LevelNormal, name, steps...)
}

func (r *Recorder) TrackHigh(name string, steps ...func() error) error {
	return r.trackLevel(LevelHigh, name, steps...)
}

func TrackHigh(name string, steps ...func() error) error {
	return DefaultRecorder().TrackHigh(name, steps...)
}
