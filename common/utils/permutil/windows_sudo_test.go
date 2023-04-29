package permutil

import (
	"os"
	"testing"
)

func TestWindowsSudo(t *testing.T) {
	var err = WindowsSudo("yak version", WithStdout(os.Stdout), WithStderr(os.Stdout))
	if err != nil {
		panic(err)
	}
}
