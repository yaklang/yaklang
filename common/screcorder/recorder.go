package screcorder

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/consts"
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

	// Context for graceful shutdown
	ctx    context.Context
	cancel context.CancelFunc

	// Stdin pipe to send commands to ffmpeg
	stdin io.WriteCloser
}

func NewScreenRecorder(config *Config, dev *ScreenDevice) (*ScreenRecorder, error) {
	if config == nil {
		config = NewDefaultConfig()
	}
	file, err := os.CreateTemp("", "yak-screen-record-*.mp4")
	if err != nil {
		return nil, err
	}

	// Create context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())

	return &ScreenRecorder{
		config:   config,
		filename: file.Name(),
		file:     file,
		device:   dev,
		ctx:      ctx,
		cancel:   cancel,
	}, nil
}

func (r *ScreenRecorder) startRecordProcess(procCtx context.Context) {
	// Build ffmpeg command manually to have better control over stdin
	ffmpegPath := consts.GetFfmpegPath()
	if ffmpegPath == "" {
		r.setError(errors.New("ffmpeg binary path is not configured"))
		return
	}

	// Get framerate from config, fallback to 24 if not set
	framerate := r.config.Framerate
	if framerate <= 0 {
		framerate = 24 // Fallback to 24fps if not configured
	}
	framerateStr := strconv.Itoa(framerate)

	// Build ffmpeg arguments based on platform
	var args []string

	if r.device.PlatformDemuxer == "avfoundation" {
		// macOS parameters - use original fast settings
		args = []string{
			"-y", // Automatically overwrite output files
			"-f", "avfoundation",
			"-r", framerateStr,
			"-i", r.device.FfmpegInputName,
			"-c:v", "libx264",
			"-preset", "ultrafast",
			"-an",                                    // No audio
			"-movflags", "+frag_keyframe+empty_moov", // Make the mp4 streamable
			r.filename,
		}
	} else if r.device.PlatformDemuxer == "gdigrab" {
		// Windows parameters - restore original ultrafast preset for speed
		args = []string{
			"-y", // Automatically overwrite output files
			"-f", "gdigrab",
			"-r", framerateStr,
			"-i", r.device.FfmpegInputName,
			"-vf", "scale=trunc(iw/2)*2:trunc(ih/2)*2,setpts=1*PTS", // Fix odd dimensions + original PTS
			"-c:v", "libx264",
			"-preset", "ultrafast", // Original Windows setting for speed
			"-pix_fmt", "yuv420p", // Keep yuv420p for compatibility
			"-an",                     // No audio
			"-movflags", "+faststart", // Standard MP4 with metadata at beginning for Windows compatibility
			r.filename,
		}
	} else {
		// Generic fallback
		args = []string{
			"-y", // Automatically overwrite output files
			"-f", r.device.PlatformDemuxer,
			"-r", framerateStr,
			"-i", r.device.FfmpegInputName,
			"-c:v", "libx264",
			"-preset", "medium",
			"-pix_fmt", "yuv420p",
			"-an",                                    // No audio
			"-movflags", "+frag_keyframe+empty_moov", // Make the mp4 streamable
			r.filename,
		}
	}

	if r.config.MouseCapture {
		if r.device.PlatformDemuxer == "avfoundation" {
			args = append(args[:len(args)-1], "-capture_cursor", "1", r.filename)
		} else if r.device.PlatformDemuxer == "gdigrab" {
			args = append(args[:len(args)-1], "-draw_mouse", "1", r.filename)
		}
	}

	cmd := exec.CommandContext(r.ctx, ffmpegPath, args...)

	// Set up stdin pipe BEFORE starting the process
	stdin, err := cmd.StdinPipe()
	if err != nil {
		r.setError(err)
		return
	}

	// Set up debug output
	cmd.Stdout = log.NewLogWriter(log.InfoLevel)
	cmd.Stderr = log.NewLogWriter(log.InfoLevel)
	log.Infof("starting ffmpeg screen recording: %s", cmd.String())

	// Start the process
	if err := cmd.Start(); err != nil {
		r.setError(err)
		return
	}

	r.cmd = cmd
	r.stdin = stdin
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
	// Call the main Stop method which handles all cleanup logic
	r.Stop()
}

func (r *ScreenRecorder) Stop() {
	r.Lock()
	defer r.Unlock()

	// Prevent multiple calls to Stop()
	if r.isStop {
		return
	}
	r.isStop = true
	r.stopTime = time.Now()
	r.recordTime = int(r.stopTime.Sub(r.startTime).Seconds())

	// Send 'q' command to ffmpeg stdin for graceful shutdown
	if r.stdin != nil {
		_, err := r.stdin.Write([]byte("q\n"))
		if err != nil {
			log.Warnf("failed to send quit command to ffmpeg: %v", err)
		}
		_ = r.stdin.Close()
		r.stdin = nil // Prevent double close
	}

	// Close file handle early
	if r.file != nil {
		_ = r.file.Close()
		r.file = nil
	}

	// Give ffmpeg some time to finish writing the file gracefully
	if r.cmd != nil && r.cmd.Process != nil {
		// Wait a bit for graceful shutdown
		timeout := time.NewTimer(5 * time.Second) // 5 seconds for Windows
		done := make(chan bool, 1)                // Buffered channel to prevent goroutine leak

		go func() {
			r.cmd.Wait()
			done <- true
		}()

		select {
		case <-done:
			// Process exited gracefully
			timeout.Stop()
			log.Infof("ffmpeg exited gracefully")
		case <-timeout.C:
			// Timeout, force kill
			log.Warnf("ffmpeg did not exit gracefully within 5 seconds, force killing")
			if r.cmd.Process != nil {
				_ = r.cmd.Process.Kill()
			}
		}
	}

	// Cancel context as backup
	if r.cancel != nil {
		r.cancel()
	}
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
		_ = r.file.Close()
		_ = os.Remove(r.file.Name())
	}

	// Clean up stdin
	if r.stdin != nil {
		_ = r.stdin.Close()
	}

	// Clean up context
	if r.cancel != nil {
		r.cancel()
	}
}
