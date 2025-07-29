package screcorder

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/ffmpegutils"
)

type ScreenRecorder struct {
	sync.Mutex
	config     *Config
	device     *ScreenDevice
	filename   string
	file       *os.File
	cmd        *exec.Cmd
	err        error
	isStop     bool
	isStarted  bool
	startTime  time.Time
	stopTime   time.Time
	recordTime int
}

func NewScreenRecorder(config *Config, dev *ScreenDevice) (*ScreenRecorder, error) {
	if config == nil {
		config = NewDefaultConfig()
	}
	file, err := os.CreateTemp("", "yak-screen-record-*.mp4")
	if err != nil {
		return nil, err
	}
	return &ScreenRecorder{
		config:   config,
		filename: file.Name(),
		file:     file,
		device:   dev,
	}, nil
}

func (r *ScreenRecorder) startRecordProcess(procCtx context.Context) {
	if procCtx == nil {
		procCtx = context.Background()
	}

	cmd, err := ffmpegutils.StartScreenRecording(r.filename,
		ffmpegutils.WithContext(procCtx),
		ffmpegutils.WithScreenRecordFormat(r.device.PlatformDemuxer),
		ffmpegutils.WithScreenRecordInput(r.device.FfmpegInputName),
		ffmpegutils.WithScreenRecordFramerate(24),
		ffmpegutils.WithScreenRecordCaptureCursor(r.config.MouseCapture),
		ffmpegutils.WithDebug(true),
	)
	if err != nil {
		r.setError(err)
		return
	}

	r.cmd = cmd
	r.startTime = time.Now()
	r.recordTime = 0

	go func() {
		err := r.cmd.Wait()
		r.stopRecord()
		if err != nil {
			// A non-zero exit code is expected when we stop the process.
			// We log only unexpected errors.
			if exitErr, ok := err.(*exec.ExitError); !ok {
				// Not an ExitError, this is an unexpected kind of error.
				log.Errorf("screen recording process finished with unexpected error: %v", err)
				r.setError(err)
			} else {
				// It is an ExitError, check if it's one of the expected signals from graceful/forceful stop.
				errMsg := exitErr.Error()
				if !strings.Contains(errMsg, "signal: killed") && !strings.Contains(errMsg, "signal: interrupt") && !strings.Contains(errMsg, "exit status 255") {
					log.Errorf("screen recording process finished with error: %v", err)
					r.setError(err)
				}
				// Otherwise, it's an expected shutdown signal, so we don't log it as an error.
			}
		}
	}()
}

func (r *ScreenRecorder) Start(ctx context.Context) error {
	r.Lock()
	defer r.Unlock()
	if r.isStarted {
		return errors.New("recorder is already started")
	}
	if r.isStop {
		return errors.New("recorder is already stopped")
	}
	r.isStarted = true

	go r.startRecordProcess(ctx)
	return nil
}

func (r *ScreenRecorder) stopRecord() {
	r.Lock()
	defer r.Unlock()
	if r.isStop {
		return
	}
	r.isStop = true
	r.stopTime = time.Now()
	r.recordTime = int(r.stopTime.Sub(r.startTime).Seconds())
	if r.file != nil {
		_ = r.file.Close()
	}
}

func (r *ScreenRecorder) Stop() {
	if r.cmd != nil && r.cmd.Process != nil {
		// Send SIGINT to ffmpeg to allow it to shut down gracefully
		// This is important for it to finalize the video file (e.g., write the moov atom)
		if err := r.cmd.Process.Signal(os.Interrupt); err != nil {
			log.Warnf("failed to send SIGINT to ffmpeg, falling back to killing the process: %v", err)
			// If sending a graceful signal fails, force kill it
			_ = r.cmd.Process.Kill()
		}
	}
	// DO NOT call r.stopRecord() here.
	// The goroutine that waits on the command will handle the cleanup
	// after the process has fully exited, preventing a race condition.
}

func (r *ScreenRecorder) IsRecording() bool {
	return r.isStarted && !r.isStop
}

func (r *ScreenRecorder) Filename() string {
	return r.filename
}

func (r *ScreenRecorder) GetFrame(frameNum int) ([]byte, error) {
	inFileName := r.filename
	if r.IsRecording() {
		return nil, errors.New("cannot get frame while recording is in progress")
	}
	if _, err := os.Stat(inFileName); os.IsNotExist(err) {
		return nil, errors.New("record file not found, maybe recording is not started yet")
	}
	return ffmpegutils.ExtractSpecificFrame(inFileName, frameNum)
}

func (r *ScreenRecorder) setError(err error) {
	r.Lock()
	defer r.Unlock()
	r.err = err
}

func (r *ScreenRecorder) GetError() error {
	r.Lock()
	defer r.Unlock()
	return r.err
}

func (r *ScreenRecorder) Close() {
	r.Stop()
	if r.file != nil {
		_ = os.Remove(r.file.Name())
	}
}
