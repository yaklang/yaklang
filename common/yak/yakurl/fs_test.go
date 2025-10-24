package yakurl_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yakgrpc"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestEscape(t *testing.T) {
	local, err := yakgrpc.NewLocalClient()
	require.NoError(t, err)

	t.Run("test ()", func(t *testing.T) {
		progName := uuid.NewString()
		defer ssadb.DeleteProgram(ssadb.GetDB(), progName)

		// fileName := "high.php"
		folderName := "a"
		folderPath := []string{progName, "vulnerabilities", "csrf", "source"}
		ssadb.MarshalFolder(append(folderPath, folderName)).Save(ssadb.GetDB())
		path := fmt.Sprintf("/%s/%s", strings.Join(folderPath, "/"), folderName)

		schema := "ssadb"
		raw := fmt.Sprintf("%s://%s", schema, path)
		res, err := local.RequestYakURL(context.Background(), &ypb.RequestYakURLParams{
			Method: "GET",
			Url: &ypb.YakURL{
				FromRaw: raw,
			},
		})
		require.NoError(t, err)
		spew.Dump(res)
	})
}

func TestEmptyPath(t *testing.T) {
	local, err := yakgrpc.NewLocalClient()
	require.NoError(t, err)

	t.Run("test empty path", func(t *testing.T) {
		res, err := local.RequestYakURL(context.Background(), &ypb.RequestYakURLParams{
			Method: "GET",
			Url: &ypb.YakURL{
				Schema: "ssadb",
				Path:   "",
			},
		})
		_ = err
		// require.Contains(t, err.Error(), "not exist")
		spew.Dump(res)
	})

}
