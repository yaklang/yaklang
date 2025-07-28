package whisperutils

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/log"
)

// WhisperManager manages a whisper-server process.
type WhisperManager struct {
	binaryPath   string
	modelPath    string
	port         int
	ctx          context.Context
	cancel       context.CancelFunc
	cmd          *exec.Cmd
	debug        bool
	enableGPU    bool
	language     string
	noContext    bool
	threads      int
	processors   int
	flashAttn    bool
	vad          bool
	vadModelPath string
}

// Option is a functional option for WhisperManager.
type Option func(*WhisperManager)

// WithEnableGPU enables GPU for the whisper server.
func WithEnableGPU(enable bool) Option {
	return func(m *WhisperManager) {
		m.enableGPU = enable
	}
}

// WithDebug enables or disables debug logging for the manager.
func WithDebug(debug bool) Option {
	return func(m *WhisperManager) {
		m.debug = debug
	}
}

// WithLanguage sets the language for the whisper server.
func WithLanguage(lang string) Option {
	return func(m *WhisperManager) {
		m.language = lang
	}
}

// WithNoContext enables or disables "no context" for the whisper server.
func WithNoContext(nc bool) Option {
	return func(m *WhisperManager) {
		m.noContext = nc
	}
}

// WithThreads sets the number of threads for the whisper server.
// This controls the CPU thread count for processing.
func WithThreads(threads int) Option {
	return func(m *WhisperManager) {
		m.threads = threads
	}
}

// WithProcessors sets the number of processors for the whisper server.
// This controls the number of parallel audio processors.
func WithProcessors(processors int) Option {
	return func(m *WhisperManager) {
		m.processors = processors
	}
}

// WithFlashAttn enables flash attention for the whisper server.
// Flash attention is a memory-efficient attention mechanism that can speed up transcription on supported hardware.
func WithFlashAttn(enable bool) Option {
	return func(m *WhisperManager) {
		m.flashAttn = enable
	}
}

// WithVAD enables Voice Activity Detection for the whisper server.
// VAD helps filter out silent parts of the audio, which can improve accuracy and speed.
func WithVAD(enable bool) Option {
	return func(m *WhisperManager) {
		m.vad = enable
	}
}

// WithVADModelPath sets the path to the Voice Activity Detection model.
// This is required if VAD is enabled.
func WithVADModelPath(path string) Option {
	return func(m *WhisperManager) {
		m.vadModelPath = path
	}
}

// WithPort sets the port for the whisper server.
func WithPort(port int) Option {
	return func(m *WhisperManager) {
		m.port = port
	}
}

// WithModelPath sets the model path for the whisper server.
func WithModelPath(path string) Option {
	return func(m *WhisperManager) {
		m.modelPath = path
	}
}

// WithContext sets the context for the whisper server.
func WithContext(ctx context.Context) Option {
	return func(m *WhisperManager) {
		m.ctx = ctx
	}
}

// NewWhisperManagerFromBinaryPath creates a new WhisperManager.
func NewWhisperManagerFromBinaryPath(binaryPath string, opts ...Option) (*WhisperManager, error) {
	manager := &WhisperManager{
		binaryPath: binaryPath,
		port:       9000, // default port
		language:   "zh",
		noContext:  true,
		threads:    4, // default number of threads
		processors: 1, // default number of audio processors
		flashAttn:  false,
		vad:        false, // VAD is disabled by default as it requires a model path
	}

	for _, opt := range opts {
		opt(manager)
	}

	if manager.port == 0 {
		return nil, fmt.Errorf("port can not be 0")
	}

	if manager.modelPath != "" {
		if _, err := os.Stat(manager.modelPath); os.IsNotExist(err) {
			return nil, fmt.Errorf("model file not found: %s", manager.modelPath)
		}
	}

	var cancel context.CancelFunc
	if manager.ctx == nil {
		manager.ctx, cancel = context.WithCancel(context.Background())
	} else {
		manager.ctx, cancel = context.WithCancel(manager.ctx)
	}
	manager.cancel = cancel

	return manager, nil
}

// Start starts the whisper-server process and waits for it to be ready.
func (m *WhisperManager) Start() error {
	args := []string{
		"--port", strconv.Itoa(m.port),
	}
	if m.modelPath != "" {
		args = append(args, "-m", m.modelPath)
	}

	if !m.enableGPU {
		args = append(args, "--no-gpu")
	}

	if m.language != "" {
		args = append(args, "-l", m.language)
	}

	if m.noContext {
		args = append(args, "-nc")
	}

	if m.threads > 0 {
		args = append(args, "-t", strconv.Itoa(m.threads))
	}

	if m.processors > 0 {
		args = append(args, "-p", strconv.Itoa(m.processors))
	}

	if m.flashAttn {
		args = append(args, "--flash-attn")
	}

	if m.vad {
		args = append(args, "--vad")
		if m.vadModelPath != "" {
			args = append(args, "--model-vad", m.vadModelPath)
		}
	}

	// ./whisper-server -l zh --no-gpu -nc -m /Users/v1ll4n/Downloads/ggml-medium-q8_0.bin
	/*
			/Users/v1ll4n/yakit-projects/projects/libs/whisper-server \
		  --port 55726 \
		  -m /Users/v1ll4n/yakit-projects/projects/libs/models/whisper-medium-q5.gguf \
		  -l zh \
		  --no-gpu \
		  --no-context \
		  -t 8 \
		  -p 2 \
		  --flash-attn \
		  --vad
	*/
	m.cmd = exec.CommandContext(m.ctx, m.binaryPath, args...)

	if m.debug {
		m.cmd.Stdout = os.Stderr
		m.cmd.Stderr = os.Stderr
	}

	log.Infof("starting whisper-server with command: %s", m.cmd.String())

	if err := m.cmd.Start(); err != nil {
		return fmt.Errorf("failed to start whisper-server: %w", err)
	}

	go func() {
		err := m.cmd.Wait()
		if err != nil {
			// Log error if process exits unexpectedly
			if m.ctx.Err() == nil {
				log.Warnf("whisper-server process exited with error: %v", err)
			} else {
				log.Infof("whisper-server process stopped as expected.")
			}
		}
	}()

	return m.waitForServerReady()
}

func (m *WhisperManager) waitForServerReady() error {
	// a simple timer to check port, max 1 minute
	timeout := time.After(1 * time.Minute)
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	once := sync.Once{}
	for {
		select {
		case <-m.ctx.Done():
			_ = m.Stop()
			return m.ctx.Err()
		case <-timeout:
			_ = m.Stop()
			return fmt.Errorf("whisper-server failed to start on port %d within 1 minute timeout", m.port)
		case <-ticker.C:
			once.Do(func() {
				log.Infof("waiting for whisper-server to be ready on port %d", m.port)
			})
			conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", m.port), 200*time.Millisecond)
			if err == nil {
				_ = conn.Close()
				log.Infof("whisper-server is ready on port %d", m.port)
				return nil
			}
		}
	}
}

// Stop stops the whisper-server process.
func (m *WhisperManager) Stop() error {
	log.Infof("stopping whisper-server...")
	if m.cancel != nil {
		m.cancel()
	}
	return nil
}

// TranscribeLocally provides a convenient way to transcribe a file using the managed server instance.
func (m *WhisperManager) TranscribeLocally(filePath string) (*TranscriptionProcessor, error) {
	client, err := NewLocalWhisperClient(WithBaseURL(fmt.Sprintf("http://127.0.0.1:%d", m.port)))
	if err != nil {
		return nil, fmt.Errorf("failed to create local whisper client: %w", err)
	}
	return client.Transcribe(filePath)
}
