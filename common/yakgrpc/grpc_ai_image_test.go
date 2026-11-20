package yakgrpc

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aimedia"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestReadAIImageSessionIsolationRecoveryAndVariants(t *testing.T) {
	ctx := context.Background()
	db, err := utils.CreateTempTestDatabaseInMemory()
	require.NoError(t, err)
	defer db.Close()
	require.NoError(t, db.AutoMigrate(&schema.AIAgentRuntime{}).Error)
	workdir := t.TempDir()
	for _, session := range []string{"session-a", "session-b"} {
		require.NoError(t, db.Create(&schema.AIAgentRuntime{Uuid: session, PersistentSession: session, WorkDir: workdir}).Error)
	}
	store, err := aimedia.OpenCatalog(aicommon.ImageCatalogDirectory(workdir, "session-a"))
	require.NoError(t, err)
	require.NoError(t, store.BindOwner("session-a"))
	var raw bytes.Buffer
	require.NoError(t, png.Encode(&raw, image.NewRGBA(image.Rect(0, 0, 800, 600))))
	img, err := aimedia.FromBytes(ctx, raw.Bytes(), "image/png")
	require.NoError(t, err)
	o, err := store.Register(ctx, "capture", img)
	require.NoError(t, err)
	server := &Server{projectDatabase: db}
	thumb, err := server.ReadAIImage(ctx, &ypb.AIImageRequest{SessionID: "session-a", ImageID: o.ID})
	require.NoError(t, err)
	require.Equal(t, int64(480), thumb.Width)
	original, err := server.ReadAIImage(ctx, &ypb.AIImageRequest{SessionID: "session-a", ImageID: o.ID, Variant: "original"})
	require.NoError(t, err)
	require.Equal(t, raw.Bytes(), original.Data)
	model, err := server.ReadAIImage(ctx, &ypb.AIImageRequest{SessionID: "session-a", ImageID: o.ID, Variant: "model"})
	require.NoError(t, err)
	require.Equal(t, int64(800), model.Width)
	_, err = server.ReadAIImage(ctx, &ypb.AIImageRequest{SessionID: "session-b", ImageID: o.ID, Scope: "session-a"})
	require.ErrorContains(t, err, "owned")
	_, err = server.ReadAIImage(ctx, &ypb.AIImageRequest{SessionID: "session-b", ImageID: o.ID})
	require.ErrorContains(t, err, "not visible")
	child, err := store.Fork(ctx, aicommon.ImageCatalogDirectory(workdir, "child-scope"))
	require.NoError(t, err)
	require.NoError(t, child.BindOwner("session-a"))
	_, err = server.ReadAIImage(ctx, &ypb.AIImageRequest{SessionID: "session-a", Scope: "child-scope", ImageID: o.ID})
	require.NoError(t, err)
	require.NoError(t, os.Remove(o.Image.Path))
	_, err = server.ReadAIImage(ctx, &ypb.AIImageRequest{SessionID: "session-a", ImageID: o.ID, Variant: "original"})
	require.Error(t, err)
}
