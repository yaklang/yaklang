package yakgrpc

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"strings"
	"testing"

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
	stream.Send(&ypb.Input{
		Raw: []byte(fmt.Sprintf("%s %s\n", testBinaryPath, temp.Name())),
	})

	passed := false

	for {
		output, err := stream.Recv()
		if err != nil {
			break
		}
		outputStr := strings.TrimSpace(string(output.Raw))
		if outputStr == testText {
			passed = true
			cancel()
		}
		t.Logf("output: %s", outputStr)
	}

	if !passed {
		t.Fatalf("failed to read expect output from terminal")
	}
}
