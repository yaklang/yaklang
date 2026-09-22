package yakgrpc

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"
)

func TestInMemoryClient(t *testing.T) {
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
