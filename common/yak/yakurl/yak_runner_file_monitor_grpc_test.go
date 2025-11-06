package yakurl_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"github.com/yaklang/yaklang/common/yak/yakurl"
	"github.com/yaklang/yaklang/common/yakgrpc"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestMonitor_MultipleFileChanges(t *testing.T) {
	// Create a temporary directory for testing
	tmpDir1, err := os.MkdirTemp("", "yak_monitor_test_1_*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir1)

	tmpDir2, err := os.MkdirTemp("", "yak_monitor_test_2_*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir2)

	local, err := yakgrpc.NewLocalClient()
	require.NoError(t, err)

	duplex, err := local.DuplexConnection(context.Background())
	require.NoError(t, err)

	id1 := uuid.NewString()
	data1 := codec.AnyToBytes(map[string]string{
		"operate": yakurl.OP_NEW_MONITOR,
		"path":    tmpDir1,
		"id":      id1,
	})
	duplex.Send(&ypb.DuplexConnectionRequest{
		MessageType: yakit.ServerPushType_File_Monitor,
		Data:        data1,
	})

	id2 := uuid.NewString()
	data2 := codec.AnyToBytes(map[string]string{
		"operate": yakurl.OP_NEW_MONITOR,
		"path":    tmpDir2,
		"id":      id2,
	})
	duplex.Send(&ypb.DuplexConnectionRequest{
		MessageType: yakit.ServerPushType_File_Monitor,
		Data:        data2,
	})

	// Forward received responses to a channel so we can select with timeout.
	respCh := make(chan *ypb.DuplexConnectionResponse, 100)
	errCh := make(chan error, 10)
	go func() {
		for {
			resp, err := duplex.Recv()
			require.NoError(t, err)
			log.Errorf("recv resp: %+v", resp)
			respCh <- resp
		}
	}()

	go func() {
		time.Sleep(1 * time.Second)
		// test path 1
		// Create a new file
		log.Errorf("write file tmp1 %v", tmpDir1)
		testFile1 := filepath.Join(tmpDir1, "test.txt")
		err = os.WriteFile(testFile1, []byte("test content"), 0644)
		require.NoError(t, err)

		log.Errorf("finish operate file 1 ")

		// test path 2
		// Create a new file
		log.Errorf("write file tmp2 %v", tmpDir2)
		testFile2 := filepath.Join(tmpDir2, "test.txt")
		err = os.WriteFile(testFile2, []byte("test content"), 0644)
		require.NoError(t, err)

		log.Errorf("finish operate file 2 ")
	}()

	seen := map[string]int{id1: 0, id2: 0}
	timeout := time.After(10 * time.Second)

	breakLoop := false
	for {
		if breakLoop {
			break
		}

		select {
		case resp := <-respCh:
			log.Errorf("resp: %+v", resp)
			if resp.MessageType != yakit.ServerPushType_File_Monitor {
				continue
			}
			eventSet := &filesys.EventSet{}
			err = json.Unmarshal(resp.GetData(), eventSet)
			require.NoError(t, err)
			log.Errorf("event: %+v", eventSet)

			if eventSet.Id == id1 {
				log.Errorf("match id1 ")
				if len(eventSet.CreateEvents) == 1 {
					log.Errorf("event1 create: %+v", eventSet.CreateEvents[0])
					require.Contains(t, eventSet.CreateEvents[0].Path, tmpDir1)
					seen[id1]++
				}
			}

			if eventSet.Id == id2 {
				if len(eventSet.CreateEvents) == 1 {
					log.Errorf("event2 create: %+v", eventSet.CreateEvents[0])
					require.Contains(t, eventSet.CreateEvents[0].Path, tmpDir2)
					seen[id2]++
				}
			}

			if seen[id1] == 1 && seen[id2] == 1 {
				// received both monitors' events; exit loop
				breakLoop = true
			}
		case err := <-errCh:
			// propagate receive errors
			require.NoError(t, err)
			breakLoop = true
		case <-timeout:
			t.Fatalf("timeout waiting for monitor events")
			breakLoop = true
		}
	}

}
