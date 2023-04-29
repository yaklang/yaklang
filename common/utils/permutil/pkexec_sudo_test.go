package permutil

import (
	"os"
	"testing"
)

func TestPKExec(t *testing.T) {
	LinuxPKExecSudo("yak version", WithStdout(os.Stdout), WithStderr(os.Stdout))
}
