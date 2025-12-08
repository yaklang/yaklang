package yakgrpc

import (
	"context"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"io"
	"strings"
	"testing"
	"time"
)

func TestMITMProcessWatcher(t *testing.T) {
	t.Skip("just local test")

	client, err := NewLocalClient(true)
	require.NoError(t, err)

	stream, err := client.WatchProcessConnection(context.Background())
	require.NoError(t, err)

	err = stream.Send(&ypb.WatchProcessRequest{
		StartParams: &ypb.WatchProcessStartParams{
			CheckIntervalSeconds: 3,
		},
	})
	require.NoError(t, err)

	for {
		status, err := stream.Recv()
		if err == io.EOF {
			return
		}
		require.NoError(t, err)

		if strings.Contains(strings.ToLower(status.Process.Name), "chrome") {
			go func() {
				for i := 0; i < 10; i++ {
					time.Sleep(time.Duration(5) * time.Second)
					stream.Send(&ypb.WatchProcessRequest{
						QueryPid: status.Process.Pid,
					})
				}
			}()
		}

		if status.Connections != nil {
			for _, conn := range status.Connections {
				t.Logf(" %s: pid [%d] name [%s]  conn [%s -> %s] domain %v\n ",
					status.Action,
					status.Process.Pid,
					status.Process.Name,
					conn.LocalAddress,
					conn.RemoteAddress,
					conn.Domain,
				)
			}
		}
		//fmt.Printf(" %s: pid [%d] name [%s] exe [%s] cmdline [%s] \n ", status.Action, status.Process.Pid, status.Process.Name, status.Process.Exe, status.Process.Cmdline)
	}

}
