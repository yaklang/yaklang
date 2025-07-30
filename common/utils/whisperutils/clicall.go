package whisperutils

import (
	"bufio"
	"context"
	"fmt"
	"github.com/yaklang/yaklang/common/utils"
	"io"
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
	binaryPath           string
	modelPath            string
	ctx                  context.Context
	cancel               context.CancelFunc
	debug                bool
	language             string
	threads              int
	processors           int
	vad                  bool
	vadModelPath         string
	vadThreshold         float64
	vadSpeechDetect      int // in ms
	outputSRT            bool
	enableGPU            bool
	filePath             string
	vadPadding           int
	splitOnWord          bool
	vadMinSpeechDuration int
	beamSize             int
	logWriter            io.Writer
	srtTargetPath        string
}

// CliOption is a functional option for configuring WhisperCli.
type CliOption func(*WhisperCli)

// WithModelPath sets the model path for transcription.
func WithModelPath(path string) CliOption {
	return func(c *WhisperCli) {
		c.modelPath = path
	}
}

// WithContext sets the context for the command.
func WithContext(ctx context.Context) CliOption {
	return func(c *WhisperCli) {
		c.ctx = ctx
	}
}

// WithDebug enables or disables debug logging for the command's output.
func WithDebug(debug bool) CliOption {
	return func(c *WhisperCli) {
		c.debug = debug
	}
}

// WithLanguage sets the language for transcription.
func WithLanguage(lang string) CliOption {
	return func(c *WhisperCli) {
		c.language = lang
	}
}

// WithThreads sets the number of CPU threads to use.
func WithThreads(threads int) CliOption {
	return func(c *WhisperCli) {
		c.threads = threads
	}
}

// WithProcessors sets the number of parallel processors.
func WithProcessors(processors int) CliOption {
	return func(c *WhisperCli) {
		c.processors = processors
	}
}

// WithVAD enables or disables Voice Activity Detection.
func WithVAD(enable bool) CliOption {
	return func(c *WhisperCli) {
		c.vad = enable
	}
}

// WithVADModelPath sets the path to the VAD model.
func WithVADModelPath(path string) CliOption {
	return func(c *WhisperCli) {
		c.vadModelPath = path
	}
}

// WithVADThreshold sets the VAD threshold.
func WithVADThreshold(threshold float64) CliOption {
	return func(c *WhisperCli) {
		c.vadThreshold = threshold
	}
}

// WithVADSpeechDetect sets the VAD speech detection duration in milliseconds.
func WithVADSpeechDetect(duration int) CliOption {
	return func(c *WhisperCli) {
		c.vadSpeechDetect = duration
	}
}

// WithVADPadding sets the VAD padding.
func WithVADPadding(padding int) CliOption {
	return func(c *WhisperCli) {
		c.vadPadding = padding
	}
}

// WithSplitOnWord enables splitting on word.
func WithSplitOnWord(enable bool) CliOption {
	return func(c *WhisperCli) {
		c.splitOnWord = enable
	}
}

// WithVADMinSpeechDuration sets the VAD min speech duration.
func WithVADMinSpeechDuration(duration int) CliOption {
	return func(c *WhisperCli) {
		c.vadMinSpeechDuration = duration
	}
}

// WithBeamSize sets the beam size.
func WithBeamSize(size int) CliOption {
	return func(c *WhisperCli) {
		c.beamSize = size
	}
}

// WithLogWriter sets the writer for non-result log lines.
func WithLogWriter(writer io.Writer) CliOption {
	return func(c *WhisperCli) {
		c.logWriter = writer
	}
}

// WithOutputSRT enables SRT file output.
func WithOutputSRT(enable bool) CliOption {
	return func(c *WhisperCli) {
		c.outputSRT = enable
	}
}

// WithEnableGPU enables or disables GPU support.
func WithEnableGPU(enable bool) CliOption {
	return func(c *WhisperCli) {
		c.enableGPU = enable
	}
}

// NewWhisperCli creates a new WhisperCli instance with the given file path and options.
func NewWhisperCli(filePath string, opts ...CliOption) (*WhisperCli, error) {
	cli := &WhisperCli{
		binaryPath:           consts.GetWhisperCliBinaryPath(),
		filePath:             filePath,
		language:             "auto",
		threads:              8,
		processors:           1,
		vadThreshold:         0.5,
		vadSpeechDetect:      300,
		enableGPU:            true,
		outputSRT:            true,
		vadPadding:           200,
		splitOnWord:          true,
		vadMinSpeechDuration: 15,
		beamSize:             2,
	}

	for _, opt := range opts {
		opt(cli)
	}

	if cli.debug && cli.logWriter == nil {
		cli.logWriter = os.Stdout
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
	if c.beamSize > 0 {
		args = append(args, "--beam-size", strconv.Itoa(c.beamSize))
	}
	if c.vadPadding > 0 {
		args = append(args, "-vp", strconv.Itoa(c.vadPadding))
	}
	if c.splitOnWord {
		args = append(args, "-sow")
	}
	if c.vadMinSpeechDuration > 0 {
		args = append(args, "-vmsd", strconv.Itoa(c.vadMinSpeechDuration))
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
			} else {
				if c.logWriter != nil {
					_, _ = fmt.Fprintln(c.logWriter, line)
				}
			}
		}
	}

	wg.Add(2)
	go processOutput(bufio.NewScanner(stdout))
	go processOutput(bufio.NewScanner(stderr))

	go func() {
		wg.Wait()
		err := cmd.Wait()
		defer close(results)
		defer func() {
			log.Info("start to checking srtTargetPath")
			if c.srtTargetPath != "" && c.outputSRT {
				// mv the generated SRT file to the target path
				// check if the SRT file exists
				outputSRT := c.filePath + ".srt"
				if !utils.FileExists(outputSRT) {
					log.Infof("cannot output srt file in %v", outputSRT)
					return
				}
				if err := os.Rename(outputSRT, c.srtTargetPath); err != nil {
					log.Warnf("failed to move SRT file to target path %s: %v", c.srtTargetPath, err)
				} else {
					log.Infof("SRT file successfully moved to: %s", c.srtTargetPath)
				}
			}
		}()
		if err != nil {
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
// It requires both the audio file path and the target SRT file path.
// The SRT file will be generated automatically by whisper-cli with the -osrt flag.
func InvokeWhisperCli(audioPath, srtTargetPath string, opts ...CliOption) (<-chan *CliResult, error) {
	// Ensure SRT output is enabled
	opts = append(opts, WithOutputSRT(true))

	if srtTargetPath == "" {
		return nil, utils.Errorf("srt target path cannot be empty")
	}

	cli, err := NewWhisperCli(audioPath, opts...)
	if err != nil {
		return nil, err
	}

	// Set the SRT target path for later use
	cli.srtTargetPath = srtTargetPath
	if cli.outputSRT {
		// check audioPath + ".srt"
		if _, err := os.Stat(audioPath + ".srt"); err == nil {
			return nil, fmt.Errorf("srt target file already exists: %s", srtTargetPath)
		}
	}

	return cli.Invoke()
}
