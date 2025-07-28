package whisperutils

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/utils"
)

func TestWhisperServerTDDUseCase(t *testing.T) {
	if utils.InGithubActions() {
		t.Skip("skipping test in github actions")
		return
	}

	/*
		whisper-server -p 9000 -m /path/to/model.bin
	*/
	binaryPath := consts.GetWhisperServerBinaryPath()
	if binaryPath == "" {
		t.Fatal("whisper-server binary not found")
	}

	randport := utils.GetRandomAvailableTCPPort()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	manager, err := NewWhisperManagerFromBinaryPath(
		binaryPath,
		WithPort(randport),
		WithModelPath(consts.GetWhisperModelPath()),
		WithContext(ctx),
		WithDebug(true),
	)
	if err != nil {
		t.Fatal(err)
	}

	err = manager.Start() // run server until port is ready
	if err != nil {
		t.Fatal(err)
	}

	ins, err := manager.TranscribeLocally(`/Users/v1ll4n/yakit-projects/projects/libs/whisper.cpp/output.wav`)
	if err != nil {
		t.Fatal(err)
	}

	srt := ins.ToSRT()
	if srt == "" {
		t.Fatal("srt is empty")
	}

	srtbysec := ins.ToSRTTeleprompter(10)
	if srtbysec == "" {
		t.Fatal("srtbysec is empty")
	}

	t.Logf("srt file content: \n%s", srt)
	fmt.Println("========================================================================")
	t.Logf("srt file content: \n%s", srtbysec)

	defer manager.Stop()
}
