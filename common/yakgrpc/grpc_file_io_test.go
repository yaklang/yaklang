package yakgrpc

import (
	"io"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestReadFile(t *testing.T) {
	client, err := NewLocalClient()
	require.NoError(t, err)
	token := utils.RandStringBytes(1024)
	tempFilePath, err := utils.SaveTempFile(token, "yak-readfile-test")
	defer os.Remove(tempFilePath)
	require.NoError(t, err)

	ctx := utils.TimeoutContextSeconds(2)
	stream, err := client.ReadFile(ctx, &ypb.ReadFileRequest{
		FilePath: tempFilePath,
		BufSize:  128,
	})
	buf := make([]byte, 0, 1024)
	require.NoError(t, err)
	for {
		res, err := stream.Recv()
		if err != nil {
			require.ErrorIs(t, err, io.EOF, "unexpected error: %v", err)
			break
		}
		buf = append(buf, res.Data...)
	}

	require.Equal(t, token, string(buf))
}
