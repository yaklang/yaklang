package log

import (
	"io"
	"os"
	"sync"
	"time"

	"github.com/pkg/errors"
	"gopkg.in/natefinch/lumberjack.v2"
)

type lumberjackRotator interface {
	io.Writer
	Rotate() error
}

type timeRotatingWriter struct {
	mu               sync.Mutex
	writer           lumberjackRotator
	rotationInterval time.Duration
	nextRotation     time.Time
	now              func() time.Time
}

func (w *timeRotatingWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	now := w.now()
	if !now.Before(w.nextRotation) {
		if err := w.writer.Rotate(); err != nil {
			return 0, err
		}
		w.nextRotation = now.Add(w.rotationInterval)
	}
	return w.writer.Write(p)
}

func InitFileRotateLogger(baseLogPath string, logSaveDay int, logRotateHour int) error {
	if logSaveDay < 0 {
		return errors.New("init file rotate logger failed: logSaveDay must not be negative")
	}
	if logRotateHour <= 0 {
		return errors.New("init file rotate logger failed: logRotateHour must be positive")
	}

	rotationInterval := time.Duration(logRotateHour) * time.Hour
	rotator := &lumberjack.Logger{
		Filename:  baseLogPath,
		MaxAge:    logSaveDay,
		LocalTime: true,
	}
	writer := &timeRotatingWriter{
		writer:           rotator,
		rotationInterval: rotationInterval,
		nextRotation:     time.Now().Add(rotationInterval),
		now:              time.Now,
	}

	w := io.MultiWriter(writer, os.Stdout)
	SetOutput(w)
	return nil
}
