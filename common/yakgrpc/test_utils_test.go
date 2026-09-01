package yakgrpc

import (
	"fmt"
	"os"
	"path"
	"testing"
	"time"

	"github.com/segmentio/ksuid"
	log "github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/netx"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"google.golang.org/grpc"
)

func NewLocalClientAndServerWithTempDatabase(t *testing.T) (ypb.YakClient, *Server, error) {
	t.Helper()
	netx.UnsetProxyFromEnv()

	lis, addr, err := newLocalGRPCListener()
	if err != nil {
		return nil, nil, err
	}
	grpcTrans := grpc.NewServer(
		grpc.MaxRecvMsgSize(100*1024*1024),
		grpc.MaxSendMsgSize(100*1024*1024),
	)
	profileDatabasePath := path.Join(os.TempDir(), fmt.Sprintf("%s.db", ksuid.New().String()))
	projectDatabasePath := path.Join(os.TempDir(), fmt.Sprintf("%s.db", ksuid.New().String()))
	s, err := newServerEx(WithInitFacadeServer(true), WithProfileDatabasePath(profileDatabasePath), WithProjectDatabasePath(projectDatabasePath))
	if err != nil {
		_ = lis.Close()
		log.Errorf("build yakit server failed: %s", err)
		return nil, nil, err
	}
	ypb.RegisterYakServer(grpcTrans, s)
	t.Cleanup(func() {
		grpcTrans.Stop()
		_ = lis.Close()
		_ = os.Remove(profileDatabasePath)
		_ = os.Remove(projectDatabasePath)
	})
	go func() {
		if serveErr := grpcTrans.Serve(lis); serveErr != nil {
			log.Debugf("temporary test gRPC server stopped: %v", serveErr)
		}
	}()
	time.Sleep(1 * time.Second)
	conn, err := grpc.Dial(addr, grpc.WithInsecure(), grpc.WithDefaultCallOptions(
		grpc.MaxCallRecvMsgSize(100*1024*1045),
		grpc.MaxCallRecvMsgSize(100*1024*1045),
	))
	if err != nil {
		return nil, nil, err
	}
	t.Cleanup(func() { _ = conn.Close() })
	client := &Client{
		YakClient: ypb.NewYakClient(conn),
		server:    s,
	}
	return client, s, nil
}

func NewLocalClientWithTempDatabase(t *testing.T) (ypb.YakClient, error) {
	t.Helper()
	client, _, err := NewLocalClientAndServerWithTempDatabase(t)
	if err != nil {
		return nil, err
	}
	return client, nil
}
