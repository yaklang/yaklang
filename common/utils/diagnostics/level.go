package diagnostics

import (
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/yaklang/yaklang/common/log"
)

const envDiagnosticsLevel = "YAK_DIAGNOSTICS_LOG_LEVEL"

type Level int

const (
	LevelLow Level = iota
	LevelNormal
	LevelHigh
	LevelOff
)

var levelNames = map[string]Level{
	"trace": LevelLow, "detail": LevelLow, "verbose": LevelLow,
	"measure": LevelNormal, "monitor": LevelNormal, "routine": LevelNormal,
	"critical": LevelHigh, "signal": LevelHigh,
	"off": LevelOff,
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
	defaultRecorder.recorder = NewRecorder()
	if raw := strings.TrimSpace(os.Getenv(envDiagnosticsLevel)); raw != "" {
		if err := SetLevelFromString(raw); err != nil {
			log.Warnf("diagnostics: ignoring invalid log level %q: %v", raw, err)
		}
	}
}

func SetLevel(lvl Level) {
	levelMu.Lock()
	level = lvl
	levelMu.Unlock()
}

func SetLevelFromString(raw string) error {
	normalized := strings.ToLower(strings.TrimSpace(raw))
	lvl, ok := levelNames[normalized]
	if !ok {
		return fmt.Errorf("unknown diagnostics log level: %s", raw)
	}
	SetLevel(lvl)
	return nil
}

func GetLevel() Level {
	levelMu.RLock()
	defer levelMu.RUnlock()
	return level
}

func (lvl Level) String() string {
	if s, ok := levelStrings[lvl]; ok {
		return s
	}
	return fmt.Sprintf("level-%d", lvl)
}

func Enabled(lvl Level) bool {
	if lvl == LevelOff {
		return false
	}
	return lvl >= GetLevel()
}

func Track(name string, steps ...func() error) error {
	return DefaultRecorder().Track(name, steps...)
}

func TrackLow(name string, steps ...func() error) error {
	return DefaultRecorder().TrackLow(name, steps...)
}

func TrackHigh(name string, steps ...func() error) error {
	return DefaultRecorder().TrackHigh(name, steps...)
}

var defaultRecorder struct {
	mu       sync.RWMutex
	recorder *Recorder
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
