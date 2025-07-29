package whisperutils

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
)

// CliResult represents a single transcribed segment from the whisper-cli tool.
type CliResult struct {
	StartTime time.Duration
	EndTime   time.Duration
	Text      string
}

// WhisperCli manages the configuration for a whisper-cli process.
type WhisperCli struct {
	binaryPath      string
	modelPath       string
	ctx             context.Context
	cancel          context.CancelFunc
	debug           bool
	language        string
	threads         int
	processors      int
	vad             bool
	vadModelPath    string
	vadThreshold    float64
	vadSpeechDetect int // in ms
	outputSRT       bool
	enableGPU       bool
	filePath        string
}

// CliOption is a functional option for configuring WhisperCli.
type CliOption func(*WhisperCli)

// CliWithModelPath sets the model path for transcription.
func CliWithModelPath(path string) CliOption {
	return func(c *WhisperCli) {
		c.modelPath = path
	}
}

// CliWithContext sets the context for the command.
func CliWithContext(ctx context.Context) CliOption {
	return func(c *WhisperCli) {
		c.ctx = ctx
	}
}

// CliWithDebug enables or disables debug logging for the command's output.
func CliWithDebug(debug bool) CliOption {
	return func(c *WhisperCli) {
		c.debug = debug
	}
}

// CliWithLanguage sets the language for transcription.
func CliWithLanguage(lang string) CliOption {
	return func(c *WhisperCli) {
		c.language = lang
	}
}

// CliWithThreads sets the number of CPU threads to use.
func CliWithThreads(threads int) CliOption {
	return func(c *WhisperCli) {
		c.threads = threads
	}
}

// CliWithProcessors sets the number of parallel processors.
func CliWithProcessors(processors int) CliOption {
	return func(c *WhisperCli) {
		c.processors = processors
	}
}

// CliWithVAD enables or disables Voice Activity Detection.
func CliWithVAD(enable bool) CliOption {
	return func(c *WhisperCli) {
		c.vad = enable
	}
}

// CliWithVADModelPath sets the path to the VAD model.
func CliWithVADModelPath(path string) CliOption {
	return func(c *WhisperCli) {
		c.vadModelPath = path
	}
}

// CliWithVADThreshold sets the VAD threshold.
func CliWithVADThreshold(threshold float64) CliOption {
	return func(c *WhisperCli) {
		c.vadThreshold = threshold
	}
}

// CliWithVADSpeechDetect sets the VAD speech detection duration in milliseconds.
func CliWithVADSpeechDetect(duration int) CliOption {
	return func(c *WhisperCli) {
		c.vadSpeechDetect = duration
	}
}

// CliWithOutputSRT enables SRT file output.
func CliWithOutputSRT(enable bool) CliOption {
	return func(c *WhisperCli) {
		c.outputSRT = enable
	}
}

// CliWithEnableGPU enables or disables GPU support.
func CliWithEnableGPU(enable bool) CliOption {
	return func(c *WhisperCli) {
		c.enableGPU = enable
	}
}

// NewWhisperCli creates a new WhisperCli instance with the given file path and options.
func NewWhisperCli(filePath string, opts ...CliOption) (*WhisperCli, error) {
	cli := &WhisperCli{
		binaryPath:      consts.GetWhisperCliBinaryPath(),
		filePath:        filePath,
		language:        "zh",
		threads:         4,
		processors:      1,
		vadThreshold:    0.5,
		vadSpeechDetect: 1000,
		enableGPU:       true,
		outputSRT:       true,
	}

	for _, opt := range opts {
		opt(cli)
	}

	if cli.modelPath == "" {
		return nil, fmt.Errorf("model path is required")
	}
	if _, err := os.Stat(cli.modelPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("model file not found: %s", cli.modelPath)
	}
	if _, err := os.Stat(cli.filePath); os.IsNotExist(err) {
		return nil, fmt.Errorf("input file not found: %s", cli.filePath)
	}
	if cli.binaryPath == "" {
		return nil, fmt.Errorf("whisper-cli binary path not configured, please check environment")
	}
	if _, err := os.Stat(cli.binaryPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("whisper-cli binary not found at: %s", cli.binaryPath)
	}

	var cancel context.CancelFunc
	if cli.ctx == nil {
		cli.ctx, cancel = context.WithCancel(context.Background())
	} else {
		cli.ctx, cancel = context.WithCancel(cli.ctx)
	}
	cli.cancel = cancel

	return cli, nil
}

// Invoke runs the whisper-cli command and streams transcription results.
func (c *WhisperCli) Invoke() (<-chan *CliResult, error) {
	args := []string{
		"-m", c.modelPath,
		"-f", c.filePath,
		"-l", c.language,
		"-t", strconv.Itoa(c.threads),
		"-p", strconv.Itoa(c.processors),
	}
	if c.vad {
		args = append(args, "--vad")
		if c.vadModelPath != "" {
			args = append(args, "--vad-model", c.vadModelPath)
		}
		args = append(args, "-vt", fmt.Sprintf("%.2f", c.vadThreshold))
		args = append(args, "-vsd", strconv.Itoa(c.vadSpeechDetect))
	}
	if !c.enableGPU {
		args = append(args, "--no-gpu")
	}
	if c.outputSRT {
		args = append(args, "-osrt")
	}

	cmd := exec.CommandContext(c.ctx, c.binaryPath, args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to get stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to get stderr pipe: %w", err)
	}

	log.Infof("invoking whisper-cli with command: %s", cmd.String())
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start whisper-cli: %w", err)
	}

	results := make(chan *CliResult)
	var wg sync.WaitGroup

	// Regex to capture timestamped transcription lines
	// [00:00:01.120 --> 00:00:05.070]   TEXT
	re := regexp.MustCompile(`^\[(\d{2}:\d{2}:\d{2}\.\d{3}) --> (\d{2}:\d{2}:\d{2}\.\d{3})\]\s*(.*)$`)

	parseDuration := func(s string) time.Duration {
		parts := strings.Split(s, ":")
		h, _ := time.ParseDuration(parts[0] + "h")
		m, _ := time.ParseDuration(parts[1] + "m")
		sec, _ := time.ParseDuration(parts[2] + "s")
		return h + m + sec
	}

	processOutput := func(scanner *bufio.Scanner) {
		defer wg.Done()
		for scanner.Scan() {
			line := scanner.Text()
			if c.debug {
				log.Debugf("whisper-cli: %s", line)
			}
			matches := re.FindStringSubmatch(line)
			if len(matches) == 4 {
				startTime := parseDuration(matches[1])
				endTime := parseDuration(matches[2])
				text := strings.TrimSpace(matches[3])
				select {
				case results <- &CliResult{StartTime: startTime, EndTime: endTime, Text: text}:
				case <-c.ctx.Done():
					return
				}
			}
		}
	}

	wg.Add(2)
	go processOutput(bufio.NewScanner(stdout))
	go processOutput(bufio.NewScanner(stderr))

	go func() {
		wg.Wait()
		close(results)
		if err := cmd.Wait(); err != nil {
			if c.ctx.Err() == nil {
				log.Warnf("whisper-cli command finished with error: %v", err)
			}
		} else {
			log.Infof("whisper-cli command finished successfully")
		}
		c.cancel()
	}()

	return results, nil
}

// InvokeWhisperCli is a convenience wrapper to create and run a WhisperCli command.
func InvokeWhisperCli(filePath string, opts ...CliOption) (<-chan *CliResult, error) {
	cli, err := NewWhisperCli(filePath, opts...)
	if err != nil {
		return nil, err
	}
	return cli.Invoke()
}
