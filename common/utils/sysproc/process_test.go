package sysproc

import (
	"fmt"
	"net"
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
