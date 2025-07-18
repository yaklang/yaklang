package yakgrpc

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestGRPCMUSTPASS_HTTPFuzzer_RenderDangerousFuzztag(t *testing.T) {
	// create a temporary file to test
	token1 := utils.RandStringBytes(16)
	fileName, err := utils.SaveTempFile(token1, "fuzztag-test-file")
	require.NoError(t, err)
	// create a codec script to test
	token2 := utils.RandStringBytes(16)
	scriptName, clearFunc, err := yakit.CreateTemporaryYakScriptEx("codec", fmt.Sprintf(`
	handle = func(origin)  {
		return "%s"
	}`, token2))
	require.NoError(t, err)
	defer clearFunc()

	pass := false

	// create a debug server
	host, port := utils.DebugMockHTTPHandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		sBody := string(body)
		if strings.Contains(sBody, token1) && strings.Contains(sBody, token2) {
			pass = true
		}
	})

	packet := fmt.Sprintf(`POST /post HTTP/1.1
Host: %s

{{file(%s)}}|{{codec(%s)}}`, utils.HostPort(host, port), fileName, scriptName)

	client, err := NewLocalClient()
	require.NoError(t, err)
	ctx, _ := context.WithTimeout(context.Background(), 80*time.Second)

	stream, err := client.HTTPFuzzer(ctx, &ypb.FuzzerRequest{
		Request:   packet,
		ForceFuzz: true,
	})

	// wait for the stream to finish
	for {
		_, err := stream.Recv()
		if err != nil {
			break
		}
	}

	require.True(t, pass, "HTTPFuzzer failed to render dangerous fuzztag")
}
