package sysproc

import (
	"fmt"
	"net"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"
)

func TestFindProcessByName(t *testing.T) {
	t.Skip("Skip TestFindProcessByName")

	port := utils.GetRandomAvailableTCPPort()
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	require.NoError(t, err)
	defer listener.Close()

	go func() {
		utils.WaitConnect(utils.HostPort("127.0.0.1", port), 3)
	}()
	fmt.Printf("Listening on http://127.0.0.1:%d\n", port)

	for {
		conn, err := listener.Accept()
		if err != nil {
			fmt.Printf("Error accepting connection: %v\n", err)
			continue
		}
		start := time.Now()
		_, processName, err := FindProcessNameByConn(conn)
		fmt.Printf("Time taken: %v\n", time.Since(start))
		if err != nil {
			fmt.Printf("Error getting connection processes: %v\n", err)
		}
		fmt.Printf("Connection Processes: %v\n", processName)
		time.Sleep(1 * time.Second)
		conn.Close()
		// listener.Close()
		// break
	}
}

// TestFindProcessNameByConn_ServerSide verifies that a server can find the owning process
// when accepting a connection. This exercises the RemoteAddr/RemotePort matching logic
// used by MITM proxy (conn.RemoteAddr = client).
func TestFindProcessNameByConn_ServerSide(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Process lookup via GetExtendedTcpTable is Windows-specific; Linux uses netlink")
	}

	port := utils.GetRandomAvailableTCPPort()
	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	require.NoError(t, err)
	defer listener.Close()

	var serverConn net.Conn
	var acceptErr error
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		serverConn, acceptErr = listener.Accept()
	}()

	clientConn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 2*time.Second)
	require.NoError(t, err)
	defer clientConn.Close()

	wg.Wait()
	require.NoError(t, acceptErr)
	require.NotNil(t, serverConn)
	defer serverConn.Close()

	// On server side: conn.RemoteAddr = client address. FindProcessNameByConn matches
	// RemoteAddr/RemotePort in TCP table to find the owning process (this test binary).
	pid, processName, err := FindProcessNameByConn(serverConn)
	require.NoError(t, err, "FindProcessNameByConn should succeed for server-accepted connection")
	require.NotZero(t, pid)
	require.NotEmpty(t, processName, "process name should not be empty")
}
