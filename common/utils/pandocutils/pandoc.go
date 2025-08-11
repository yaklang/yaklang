package pandocutils

import (
	"context"
	"fmt"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"os"
	"os/exec"
)

func SimpleCovertMarkdownToDocx(ctx context.Context, inputFile string, outputFile string) error {

	if _, err := os.Stat(inputFile); os.IsNotExist(err) {
		return fmt.Errorf("input file does not exist: %s", inputFile)
	}
	pandocBinaryPath := consts.GetPandocPath()
	if pandocBinaryPath == "" {
		return fmt.Errorf("pandoc binary path is not configured")
	}

	args := []string{
		inputFile,
		"-o",
		outputFile,
	}

	r, w := utils.NewPipe()
	defer func() {
		w.Close()
	}()
	cmd := exec.CommandContext(ctx, pandocBinaryPath, args...)
	cmd.Stderr = w
	cmd.Stdout = w
	log.Infof("executing audio extraction: %s", cmd.String())
	err := cmd.Run()
	if err != nil {
		output, _ := utils.ReadTimeout(r, 5)
		return fmt.Errorf("pandoc execution failed: %w, combined output: \n%v", err, string(output))
	}

	if !utils.FileExists(outputFile) {
		output, _ := utils.ReadTimeout(r, 5)
		log.Errorf("output file does not exist after conversion: %s, combined output: \n%v", outputFile, string(output))
		return fmt.Errorf("output file does not exist after conversion: %s", outputFile)
	}

	return nil
}
