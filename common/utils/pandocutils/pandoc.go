package pandocutils

import (
	"context"
	"fmt"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"os"
	"os/exec"
	"sync"
)

var (
	// ffmpegBinaryPath holds the path to the ffmpeg executable.
	// It is initialized by checking the system's configuration.
	pandocBinaryPath  = consts.GetPandocPath()
	deprecatedWarning = new(sync.Once)
)

func SimpleCovertMarkdownToDocx(ctx context.Context, inputFile string, outputFile string) error {
	deprecatedWarning.Do(func() {
		log.Warnf("SimpleCovertMarkdownToDocx is deprecated, please use pandoc.SimpleConvertMarkdownToDocxContext or SimpleConvertMarkdownTo instead")
	})
	if _, err := os.Stat(inputFile); os.IsNotExist(err) {
		return fmt.Errorf("input file does not exist: %s", inputFile)
	}
	if pandocBinaryPath == "" {
		return fmt.Errorf("pandoc binary path is not configured")
	}

	args := []string{
		inputFile,
		"-o",
		outputFile,
	}

	cmd := exec.CommandContext(ctx, pandocBinaryPath, args...)
	cmd.Stderr = log.NewLogWriter(log.DebugLevel)
	log.Infof("executing audio extraction: %s", cmd.String())

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("pandoc execution failed: %w", err)
	}

	return nil
}
