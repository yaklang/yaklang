package yakgrpc

import (
	"context"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yak/yaklib"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"
)

func TestInMemoryClient(t *testing.T) {
	t.Run("close waits for persistence", testInMemoryClientCloseWaitsForHandlers)
	// Every instance owns its transport and database, even when created concurrently.
	for i := 0; i < 4; i++ {
		t.Run("isolated", func(t *testing.T) {
			t.Parallel()
			db := newOnlineTestDB(t, &schema.HotPatchTemplate{}, &schema.Payload{})
			client, err := newInMemoryClient(&Server{profileDatabase: db})
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, client.Close()) })
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_, err = client.CreateHotPatchTemplate(ctx, &ypb.HotPatchTemplate{Name: "same-name", Type: "test", Content: "body"})
			require.NoError(t, err)
			var count int
			require.NoError(t, db.Model(&schema.HotPatchTemplate{}).Count(&count).Error)
			require.Equal(t, 1, count)
			stream, err := client.UploadPayloadToOnline(ctx, &ypb.UploadPayloadToOnlineRequest{Token: "token", Group: "empty"})
			require.NoError(t, err)
			var messages []*ypb.DownloadProgress
			for {
				m, err := stream.Recv()
				if err == io.EOF {
					break
				}
				require.NoError(t, err)
				messages = append(messages, m)
			}
			require.Len(t, messages, 2)
			require.Equal(t, 1.0, messages[1].Progress)
			canceled, stop := context.WithCancel(ctx)
			stop()
			_, err = client.CreateHotPatchTemplate(canceled, &ypb.HotPatchTemplate{})
			require.Equal(t, codes.Canceled, grpcstatus.Code(err))
			expired, stop := context.WithDeadline(ctx, time.Now().Add(-time.Second))
			defer stop()
			_, err = client.CreateHotPatchTemplate(expired, &ypb.HotPatchTemplate{})
			require.Equal(t, codes.DeadlineExceeded, grpcstatus.Code(err))
			require.NoError(t, client.Close())
			_, err = client.CreateHotPatchTemplate(ctx, &ypb.HotPatchTemplate{})
			require.Error(t, err)
		})
	}
}

func BenchmarkInMemoryClient(b *testing.B) {
	for i := 0; i < b.N; i++ {
		client, err := newInMemoryClient(&Server{})
		if err != nil {
			b.Fatal(err)
		}
		if err := client.Close(); err != nil {
			b.Fatal(err)
		}
	}
}

func testInMemoryClientCloseWaitsForHandlers(t *testing.T) {
	db := newOnlineTestDB(t, &schema.HotPatchTemplate{})
	entered, release := make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	client, err := newInMemoryClient(&Server{profileDatabase: db, onlineClient: &stubOnlineService{
		downloadTemplate: func(token, name, kind string) (*yaklib.HotPatchTemplate, error) {
			close(entered)
			<-release // This remote API does not accept cancellation; persistence follows it.
			return &yaklib.HotPatchTemplate{Name: name, TemplateType: kind, Content: "downloaded"}, nil
		},
	}})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	t.Cleanup(unblock)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	rpcCtx, cancelRPC := context.WithCancel(ctx)
	defer cancelRPC()
	reply := make(chan error, 1)
	go func() {
		_, err := client.DownloadHotPatchTemplate(rpcCtx, &ypb.DownloadHotPatchTemplateRequest{Token: "token", Name: "pending", Type: "test"})
		reply <- err
	}()
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal("RPC did not enter remote download")
	}
	cancelRPC()
	select {
	case err := <-reply:
		require.Equal(t, codes.Canceled, grpcstatus.Code(err))
	case <-ctx.Done():
		t.Fatal("RPC did not cancel")
	}
	closed := make(chan struct{})
	var closeErr, queryErr, databaseCloseErr error
	var saved schema.HotPatchTemplate
	go func() {
		closeErr = client.Close()
		// Match the temp-database helper's ownership order, inspecting persistence
		// immediately before disposal to catch late or failed handler writes.
		queryErr = db.Where("name = ?", "pending").First(&saved).Error
		databaseCloseErr = db.Close()
		close(closed)
	}()
	select {
	case <-closed:
		t.Error("cleanup overtook the active handler")
	case <-time.After(100 * time.Millisecond): // Channels, not this timeout, hold the handler.
	}
	unblock()
	select {
	case <-closed:
	case <-ctx.Done():
		t.Fatal("Close did not join handler")
	}
	require.NoError(t, closeErr)
	require.NoError(t, queryErr)
	require.Equal(t, "downloaded", saved.Content)
	require.NoError(t, databaseCloseErr)
	require.Error(t, db.DB().Ping())
}
