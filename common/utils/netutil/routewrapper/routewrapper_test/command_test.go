//go:build windows
// +build windows

package routewrapper_test

import (
	"bytes"
	"testing"

	"github.com/yaklang/yaklang/common/utils/netutil/routewrapper"
)

func TestCommandSpec_Run_Windows(t *testing.T) {
	cmdSpec := routewrapper.CommandSpec{
		Name: "cmd",
		Args: []string{"/C", "echo", "hello"},
	}

	stdout, stderr, err := cmdSpec.Run()

	if err != nil {
		t.Fatalf("Expected no error, but got: %v", err)
	}

	if len(stderr) > 0 {
		t.Fatalf("Expected no stderr, but got: %s", string(stderr))
	}

	expectedOutput := "hello\r\n"
	if !bytes.Equal(stdout, []byte(expectedOutput)) {
		t.Fatalf("Expected stdout to be %q, but got %q", expectedOutput, string(stdout))
	}
}
