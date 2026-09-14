package log

import (
	"errors"
	"testing"
	"time"
)

type fakeLumberjackRotator struct {
	rotations int
	writes    int
	rotateErr error
}

func (f *fakeLumberjackRotator) Write(p []byte) (int, error) {
	f.writes++
	return len(p), nil
}

func (f *fakeLumberjackRotator) Rotate() error {
	f.rotations++
	return f.rotateErr
}

func TestTimeRotatingWriter(t *testing.T) {
	now := time.Date(2026, 9, 1, 8, 0, 0, 0, time.Local)
	rotator := &fakeLumberjackRotator{}
	writer := &timeRotatingWriter{
		writer:           rotator,
		rotationInterval: time.Hour,
		nextRotation:     now.Add(time.Hour),
		now:              func() time.Time { return now },
	}

	if _, err := writer.Write([]byte("before")); err != nil {
		t.Fatal(err)
	}
	if rotator.rotations != 0 || rotator.writes != 1 {
		t.Fatalf("unexpected pre-deadline calls: rotations=%d writes=%d", rotator.rotations, rotator.writes)
	}

	now = now.Add(time.Hour)
	if _, err := writer.Write([]byte("after")); err != nil {
		t.Fatal(err)
	}
	if rotator.rotations != 1 || rotator.writes != 2 {
		t.Fatalf("rotation deadline not enforced: rotations=%d writes=%d", rotator.rotations, rotator.writes)
	}
}

func TestTimeRotatingWriterPropagatesRotateError(t *testing.T) {
	want := errors.New("rotate failed")
	rotator := &fakeLumberjackRotator{rotateErr: want}
	writer := &timeRotatingWriter{
		writer:           rotator,
		rotationInterval: time.Hour,
		nextRotation:     time.Time{},
		now:              time.Now,
	}

	if _, err := writer.Write([]byte("data")); !errors.Is(err, want) {
		t.Fatalf("got %v, want %v", err, want)
	}
	if rotator.writes != 0 {
		t.Fatalf("write proceeded after rotate error: %d", rotator.writes)
	}
}
