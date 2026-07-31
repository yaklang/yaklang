package yakgrpc

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func newLegacyPayloadDeleteTestServer(t *testing.T) *Server {
	t.Helper()

	db, err := utils.CreateTempTestDatabaseInMemory()
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&schema.Payload{}).Error)
	t.Cleanup(func() {
		require.NoError(t, db.Close())
	})
	return &Server{profileDatabase: db}
}

func requirePayloadGroupDeleted(t *testing.T, server *Server, group string) {
	t.Helper()

	require.Zero(t, yakit.GetPayloadCountInGroup(server.GetProfileDatabase(), group))
}

func TestDeletePayloadByGroupStorageModes(t *testing.T) {
	t.Run("legacy misclassified lines", func(t *testing.T) {
		server := newLegacyPayloadDeleteTestServer(t)
		group := "legacy-delete"
		for _, line := range []string{"aaaaaa", "bbbbbb"} {
			require.NoError(t, yakit.CreateOrUpdatePayload(
				server.GetProfileDatabase(),
				strconv.Quote(line),
				group,
				"",
				0,
				true,
			))
		}

		_, err := server.DeletePayloadByGroup(context.Background(), &ypb.DeletePayloadByGroupRequest{Group: group})
		require.NoError(t, err)
		requirePayloadGroupDeleted(t, server, group)
	})

	t.Run("database group", func(t *testing.T) {
		server := newLegacyPayloadDeleteTestServer(t)
		group := "database-delete"
		require.NoError(t, yakit.CreateOrUpdatePayload(
			server.GetProfileDatabase(),
			strconv.Quote("payload"),
			group,
			"",
			0,
			false,
		))

		_, err := server.DeletePayloadByGroup(context.Background(), &ypb.DeletePayloadByGroupRequest{Group: group})
		require.NoError(t, err)
		requirePayloadGroupDeleted(t, server, group)
	})

	t.Run("file-backed group", func(t *testing.T) {
		server := newLegacyPayloadDeleteTestServer(t)
		group := "file-delete"
		backingFile := filepath.Join(t.TempDir(), "payload.txt")
		require.NoError(t, os.WriteFile(backingFile, []byte("payload\n"), 0o600))
		require.NoError(t, yakit.CreateOrUpdatePayload(
			server.GetProfileDatabase(),
			backingFile,
			group,
			"",
			0,
			true,
		))

		_, err := server.DeletePayloadByGroup(context.Background(), &ypb.DeletePayloadByGroupRequest{Group: group})
		require.NoError(t, err)
		requirePayloadGroupDeleted(t, server, group)
		_, statErr := os.Stat(backingFile)
		require.ErrorIs(t, statErr, os.ErrNotExist)
	})

	t.Run("missing backing file", func(t *testing.T) {
		server := newLegacyPayloadDeleteTestServer(t)
		group := "missing-file-delete"
		backingFile := filepath.Join(t.TempDir(), "missing.txt")
		require.NoError(t, yakit.CreateOrUpdatePayload(
			server.GetProfileDatabase(),
			backingFile,
			group,
			"",
			0,
			true,
		))

		_, err := server.DeletePayloadByGroup(context.Background(), &ypb.DeletePayloadByGroupRequest{Group: group})
		require.NoError(t, err)
		requirePayloadGroupDeleted(t, server, group)
	})

	t.Run("inconsistent group is preserved", func(t *testing.T) {
		server := newLegacyPayloadDeleteTestServer(t)
		group := "inconsistent-delete"
		backingFile := filepath.Join(t.TempDir(), "payload.txt")
		require.NoError(t, os.WriteFile(backingFile, []byte("payload\n"), 0o600))
		require.NoError(t, yakit.CreateOrUpdatePayload(
			server.GetProfileDatabase(),
			backingFile,
			group,
			"",
			0,
			true,
		))
		require.NoError(t, yakit.CreateOrUpdatePayload(
			server.GetProfileDatabase(),
			strconv.Quote("database payload"),
			group,
			"",
			0,
			false,
		))

		_, err := server.DeletePayloadByGroup(context.Background(), &ypb.DeletePayloadByGroupRequest{Group: group})
		require.ErrorContains(t, err, "inconsistent storage records")
		require.EqualValues(t, 2, yakit.GetPayloadCountInGroup(server.GetProfileDatabase(), group))
		_, statErr := os.Stat(backingFile)
		require.NoError(t, statErr)
	})
}
