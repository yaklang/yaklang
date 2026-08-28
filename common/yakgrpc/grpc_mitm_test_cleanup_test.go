package yakgrpc

import (
	"context"
	"net"
	"path/filepath"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/test/bufconn"
)

// isolateMITMTestSideEffects gives an MITM integration test its own gRPC
// server and project database. A package-wide NewLocalClient server is unsafe
// here: MITM persists flows asynchronously, so rebinding only the global
// project database can race a writer left by another test. Keeping the server,
// writer and reader on one immutable database also keeps rows and sidecars out
// of the developer's active project.
func isolateMITMTestSideEffects(t *testing.T) ypb.YakClient {
	t.Helper()
	previous := consts.CaptureProjectDatabaseBinding()
	testRoot := t.TempDir()
	t.Setenv("YAKIT_HOME", testRoot)

	projectPath := filepath.Join(testRoot, "project.db")
	profilePath := filepath.Join(testRoot, "profile.db")
	server, err := newServerEx(
		WithInitFacadeServer(false),
		WithProjectDatabasePath(projectPath),
		WithProfileDatabasePath(profilePath),
	)
	if err != nil {
		t.Fatalf("create isolated MITM test server: %v", err)
	}
	// Keep the isolated database equivalent to a migrated user project. Some
	// deletion paths query columns (for example websocket_hash) that are not
	// needed by the basic MITM insert path and can otherwise be absent here,
	// silently skipping cache/WebSocket cleanup coverage.
	if err := server.projectDatabase.AutoMigrate(&schema.HTTPFlow{}).Error; err != nil {
		t.Fatalf("migrate isolated HTTPFlow schema: %v", err)
	}
	consts.BindProjectDatabaseWithReader(server.projectDatabase, server.projectReadDatabase, projectPath)

	listener := bufconn.Listen(128 * 1024 * 1024)
	grpcServer := grpc.NewServer(
		grpc.MaxRecvMsgSize(100*1024*1024),
		grpc.MaxSendMsgSize(100*1024*1024),
	)
	ypb.RegisterYakServer(grpcServer, server)
	go func() { _ = grpcServer.Serve(listener) }()

	var conn *grpc.ClientConn
	t.Cleanup(func() {
		deadline := time.Now().Add(2 * time.Second)
		for len(yakit.DBSaveAsyncChannel) > 0 && time.Now().Before(deadline) {
			time.Sleep(25 * time.Millisecond)
		}
		consts.BindProjectDatabaseWithReader(previous.Database, previous.ReadDatabase, previous.Path)
		if conn != nil {
			_ = conn.Close()
		}
		grpcServer.Stop()
		_ = listener.Close()
		if server.projectReadDatabase != nil && server.projectReadDatabase != server.projectDatabase {
			_ = server.projectReadDatabase.DB().Close()
		}
		if server.projectDatabase != nil {
			_ = server.projectDatabase.DB().Close()
		}
		if server.profileDatabase != nil {
			_ = server.profileDatabase.DB().Close()
		}
	})

	dialCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, err = grpc.DialContext(
		dialCtx,
		"bufconn",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return listener.Dial()
		}),
		grpc.WithInsecure(),
		grpc.WithBlock(),
		grpc.WithDefaultCallOptions(
			grpc.MaxCallRecvMsgSize(100*1024*1024),
			grpc.MaxCallSendMsgSize(100*1024*1024),
		),
	)
	if err != nil {
		t.Fatalf("dial isolated MITM test server: %v", err)
	}
	return ypb.NewYakClient(conn)
}

// registerHTTPFlowTokenCleanup installs the cleanup before test traffic is
// emitted. It removes MITM and WebFuzzer rows alike and waits through a short
// quiet window so an already-dequeued asynchronous insert cannot repopulate a
// row after cleanup returned.
func registerHTTPFlowTokenCleanup(t *testing.T, tokens ...string) {
	t.Helper()
	db := consts.GetGormProjectDatabase()
	t.Cleanup(func() {
		deadline := time.Now().Add(2 * time.Second)
		quietSince := time.Time{}
		for time.Now().Before(deadline) {
			found := false
			for _, token := range tokens {
				if token == "" {
					continue
				}
				filter := &ypb.QueryHTTPFlowRequest{SearchURL: token}
				var count int
				if err := yakit.FilterHTTPFlow(db.Model(&schema.HTTPFlow{}), filter).Count(&count).Error; err == nil && count > 0 {
					found = true
					_ = yakit.DeleteHTTPFlow(db, &ypb.DeleteHTTPFlowRequest{Filter: filter})
				}
			}

			if found || len(yakit.DBSaveAsyncChannel) > 0 {
				quietSince = time.Time{}
			} else if quietSince.IsZero() {
				quietSince = time.Now()
			} else if time.Since(quietSince) >= 300*time.Millisecond {
				return
			}
			time.Sleep(25 * time.Millisecond)
		}

		// Best-effort final sweep if unrelated asynchronous work kept the global
		// queue busy for the entire deadline.
		for _, token := range tokens {
			if token != "" {
				_ = yakit.DeleteHTTPFlow(db, &ypb.DeleteHTTPFlowRequest{
					Filter: &ypb.QueryHTTPFlowRequest{SearchURL: token},
				})
			}
		}
	})
}
