package yakgrpc

import (
	"context"
	"fmt"
	"io"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
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

func TestReadFileWith_SSADB(t *testing.T) {
	code := `
	print("a")
	`
	programName := uuid.NewString()
	vf := filesys.NewVirtualFs()
	vf.AddFile("a/b/c.yak", code)
	_, err := ssaapi.ParseProjectWithFS(vf,
		ssaapi.WithLanguage(ssaconfig.Yak),
		ssaapi.WithProgramName(programName),
	)
	defer ssadb.DeleteProgram(ssadb.GetDB(), programName)
	assert.NoError(t, err)

	local, err := NewLocalClient()
	assert.NoError(t, err)
	stream, err := local.ReadFile(context.Background(), &ypb.ReadFileRequest{
		FilePath:   fmt.Sprintf("/%s/a/b/c.yak", programName),
		BufSize:    128,
		FileSystem: "ssadb",
	})
	assert.NoError(t, err)

	res, err := stream.Recv()
	if err != nil {
		assert.ErrorIs(t, err, io.EOF, "unexpected error: %v", err)
	} else {
		assert.Equal(t, code, string(res.Data))
	}
}
