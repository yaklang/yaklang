package yakgrpc

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"strings"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"gotest.tools/v3/assert"
)

func TestTerminal(t *testing.T) {
	testText := uuid.NewString()
	testBinaryPath := "cat"
	if runtime.GOOS == "windows" {
		testBinaryPath = "type"
	}

	temp, err := os.CreateTemp("", "testfile")
	assert.NilError(t, err)
	temp.WriteString(testText)
	defer temp.Close()

	client, err := NewLocalClient()
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stream, err := client.YaklangTerminal(ctx)
	require.NoError(t, err)

	stream.Send(&ypb.Input{
		Path: "",
	})

	passed := false
	firstMsgRecv := false

	for {
		output, err := stream.Recv()
		if err != nil {
			break
		}
		outputStr := strings.TrimSpace(string(output.Raw))
		if !firstMsgRecv {
			firstMsgRecv = true
			stream.Send(&ypb.Input{
				Raw: []byte(fmt.Sprintf("%s %s", testBinaryPath, temp.Name())),
			})
			if runtime.GOOS == "windows" {
				stream.Send(&ypb.Input{
					Raw: []byte("\r\n"),
				})
			} else {
				stream.Send(&ypb.Input{
					Raw: []byte("\n"),
				})
			}
		}
		if strings.Contains(outputStr, testText) {
			passed = true
			cancel()
		}
		t.Logf("%s", spew.Sdump(output.Raw))
	}

	if !passed {
		t.Fatalf("failed to read expect output from terminal")
	}
}

func TestTerminalControlChar(t *testing.T) {
	testCommand := "something command"
	client, err := NewLocalClient()
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stream, err := client.YaklangTerminal(ctx)
	require.NoError(t, err)

	passed := false
	firstMsgRecv := false
	prompt := ""
	stream.Send(&ypb.Input{
		Path: "",
	})

	for {
		output, err := stream.Recv()
		if err != nil {
			break
		}
		outputStr := strings.TrimSpace(string(output.Raw))
		if !firstMsgRecv {
			firstMsgRecv = true
			prompt = outputStr
			stream.Send(&ypb.Input{
				Raw: []byte(testCommand),
			})
			stream.Send(&ypb.Input{
				Raw: []byte{3}, // Ctrl+C
			})
		} else if prompt != "" && strings.Contains(outputStr, prompt) {
			passed = true
			cancel()
		}
		t.Logf("%s", spew.Sdump(output.Raw))
	}

	if !passed {
		t.Fatalf("failed to read expect control char output from terminal")
	}
}

func TestTerminalPath(t *testing.T) {
	testText := uuid.NewString()
	testBinaryPath := "cat"
	if runtime.GOOS == "windows" {
		testBinaryPath = "type"
	}

	// os.TempDir()
	filename := "testfile"
	path := os.TempDir()
	temp, err := os.CreateTemp(path, filename)
	assert.NilError(t, err)
	temp.WriteString(testText)
	defer temp.Close()

	client, err := NewLocalClient()
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stream, err := client.YaklangTerminal(ctx)
	require.NoError(t, err)

	stream.Send(&ypb.Input{
		Path: path,
	})

	passed := false
	firstMsgRecv := false

	for {
		output, err := stream.Recv()
		if err != nil {
			spew.Dump(err)
			break
		}
		outputStr := strings.TrimSpace(string(output.Raw))
		if !firstMsgRecv {
			firstMsgRecv = true
			stream.Send(&ypb.Input{
				Raw: []byte(fmt.Sprintf("%s %s", testBinaryPath, temp.Name())),
			})
			if runtime.GOOS == "windows" {
				stream.Send(&ypb.Input{
					Raw: []byte("\r\n"),
				})
			} else {
				stream.Send(&ypb.Input{
					Raw: []byte("\n"),
				})
			}
		}
		if strings.Contains(outputStr, testText) {
			passed = true
			cancel()
		}
		t.Logf("%s", spew.Sdump(output.Raw))
	}

	if !passed {
		t.Fatalf("failed to read expect output from terminal")
	}

}
