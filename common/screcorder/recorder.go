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
	"github.com/yaklang/yaklang/common/subprocess"
	"github.com/yaklang/yaklang/common/utils/ffmpegutils"
)

type ScreenRecorder struct {
	sync.Mutex
	config     *Config
	device     *ScreenDevice
	filename   string
	file       *os.File
	mp         *subprocess.ManagedProcess
	stdin      io.WriteCloser
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
	ffmpegPath := consts.GetFfmpegPath()
	if ffmpegPath == "" {
		r.setError(errors.New("ffmpeg binary path is not configured"))
		return
	}

	framerate := r.config.Framerate
	if framerate <= 0 {
		framerate = 24
	}
	framerateStr := strconv.Itoa(framerate)

	var args []string

	if r.device.PlatformDemuxer == "avfoundation" {
		args = []string{
			"-y",
			"-f", "avfoundation",
			"-r", framerateStr,
			"-i", r.device.FfmpegInputName,
			"-c:v", "libx264",
			"-preset", "ultrafast",
			"-an",
			"-movflags", "+frag_keyframe+empty_moov",
			r.filename,
		}
	} else if r.device.PlatformDemuxer == "gdigrab" {
		args = []string{
			"-y",
			"-f", "gdigrab",
			"-r", framerateStr,
			"-i", r.device.FfmpegInputName,
			"-vf", "scale=trunc(iw/2)*2:trunc(ih/2)*2,setpts=1*PTS",
			"-c:v", "libx264",
			"-preset", "ultrafast",
			"-pix_fmt", "yuv420p",
			"-an",
			"-movflags", "+faststart",
			r.filename,
		}
	} else {
		args = []string{
			"-y",
			"-f", r.device.PlatformDemuxer,
			"-r", framerateStr,
			"-i", r.device.FfmpegInputName,
			"-c:v", "libx264",
			"-preset", "medium",
			"-pix_fmt", "yuv420p",
			"-an",
			"-movflags", "+frag_keyframe+empty_moov",
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

	// Create an io.Pipe for stdin so we can send the 'q' quit command later.
	stdinReader, stdinWriter := io.Pipe()

	log.Infof("starting ffmpeg screen recording: %s %s", ffmpegPath, strings.Join(args, " "))

	mp, err := subprocess.Launch(procCtx, &subprocess.LaunchConfig{
		Cmd: &exec.Cmd{
			Path: ffmpegPath,
			Args: append([]string{ffmpegPath}, args...),
		},
		Stdio: subprocess.StdioConfig{
			Stdin:  stdinReader,
			Stdout: log.NewLogWriter(log.InfoLevel),
			Stderr: log.NewLogWriter(log.InfoLevel),
		},
		ShutdownTimeout: 5 * time.Second,
		GracefulShutdown: func(p *subprocess.ManagedProcess) error {
			if _, err := stdinWriter.Write([]byte("q\n")); err != nil {
				log.Warnf("failed to send quit command to ffmpeg: %v", err)
			}
			_ = stdinWriter.Close()
			return nil
		},
	})
	if err != nil {
		_ = stdinWriter.Close()
		r.setError(err)
		return
	}

	r.Lock()
	r.mp = mp
	r.stdin = stdinWriter
	r.startTime = time.Now()
	r.recordTime = 0
	r.Unlock()

	// Wait for the process to exit and handle errors.
	go func() {
		<-mp.Done()
		err := mp.WaitError()
		if err != nil {
			if exitErr, ok := err.(*exec.ExitError); !ok {
				log.Errorf("screen recording process finished with unexpected error: %v", err)
				r.setError(err)
			} else {
				errMsg := exitErr.Error()
				if !strings.Contains(errMsg, "signal: killed") && !strings.Contains(errMsg, "signal: interrupt") && !strings.Contains(errMsg, "exit status 255") {
					log.Errorf("screen recording process finished with error: %v", err)
					r.setError(err)
				}
			}
		}
		r.stopRecord()
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
	r.Stop()
}

func (r *ScreenRecorder) Stop() {
	r.Lock()
	defer r.Unlock()

	if r.isStop {
		return
	}
	r.isStop = true
	r.stopTime = time.Now()
	r.recordTime = int(r.stopTime.Sub(r.startTime).Seconds())

	mp := r.mp
	r.mp = nil
	r.stdin = nil

	// Close file handle early
	if r.file != nil {
		_ = r.file.Close()
		r.file = nil
	}

	r.Unlock()
	// Close gracefully shuts down the process: sends 'q' via stdin,
	// waits up to 5 seconds, then force-kills the process group.
	if mp != nil {
		mp.Close()
	}
	r.Lock()
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
	if r.stdin != nil {
		_ = r.stdin.Close()
	}
}
