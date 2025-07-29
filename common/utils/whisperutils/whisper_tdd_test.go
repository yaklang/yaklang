package whisperutils

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/utils"
)

func TestWhisperManager(t *testing.T) {
	// This test requires a local whisper-server setup and models.
	// You need to set environment variables:
	// YAK_WHISPER_SERVER_PATH
	// YAK_WHISPER_MODEL_PATH
	serverPath := consts.GetWhisperServerBinaryPath()
	if serverPath == "" {
		t.Skip("skipping test: YAK_WHISPER_SERVER_PATH not set")
	}
	modelPath := consts.GetWhisperModelMediumPath()
	if modelPath == "" {
		t.Skip("skipping test: YAK_WHISPER_MODEL_PATH not set")
	}

	port := utils.GetRandomAvailableTCPPort()
	manager, err := NewWhisperManagerFromBinaryPath(serverPath,
		WithModelPath(modelPath),
		WithDebug(true),
		WithPort(port),
	)
	if err != nil {
		t.Fatalf("failed to create whisper manager: %v", err)
	}
	if err := manager.Start(); err != nil {
		t.Fatalf("failed to start whisper manager: %v", err)
	}
	defer manager.Stop()
}

func TestInvokeWhisperCli(t *testing.T) {
	// This test requires a local whisper-cli setup and models.
	// 1. Download whisper-cli binary and place it in a searchable path or set YAK_WHISPER_CLI_PATH.
	// 2. Download a whisper model (e.g., ggml-medium-q5.gguf) and set YAK_WHISPER_MODEL_PATH.
	// 3. Download the silero VAD model and set YAK_WHISPER_VAD_MODEL_PATH if using VAD.
	modelPath := consts.GetWhisperModelMediumPath()
	if modelPath == "" || !utils.FileExists(modelPath) {
		t.Skip("skipping test: YAK_WHISPER_MODEL_PATH is not set or model file not found")
	}

	vadModelPath := consts.GetWhisperSileroVADPath()
	if vadModelPath == "" || !utils.FileExists(vadModelPath) {
		t.Skip("skipping test: YAK_WHISPER_VAD_MODEL_PATH is not set or VAD model file not found")
	}

	audioFile := "/Users/v1ll4n/yakit-projects/projects/libs/output.mp3"
	if !utils.FileExists(audioFile) {
		t.Fatalf("test audio file not found: %s", audioFile)
	}

	results, err := InvokeWhisperCli(audioFile,
		CliWithModelPath(modelPath),
		CliWithVAD(true),
		CliWithVADModelPath(vadModelPath),
		CliWithDebug(true),
		CliWithLogWriter(os.Stdout),
	)
	if err != nil {
		t.Fatalf("InvokeWhisperCli failed: %v", err)
	}

	for res := range results {
		fmt.Printf("%v [%s -> %s] %s\n", time.Now(), res.StartTime, res.EndTime, res.Text)
	}
}
