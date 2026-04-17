package permutil

import (
	"os"
	"runtime"
	"testing"
)

func TestWindowsSudo(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("manual Windows-only test")
	}
	if os.Getenv("YAK_RUN_MANUAL_WINDOWS_SUDO_TEST") != "1" {
		t.Skip("set YAK_RUN_MANUAL_WINDOWS_SUDO_TEST=1 to run manual UAC test")
	}
	var err = WindowsSudo("yak version", WithStdout(os.Stdout), WithStderr(os.Stdout))
	if err != nil {
		panic(err)
	}
}
