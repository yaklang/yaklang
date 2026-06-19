package yaklib

import (
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"io"
	"sync"
)

type portForward struct {
}

// Forward 启动一个 TCP 端口转发，将本地端口的流量转发到远端主机端口
// 参数:
//   - localPort: 本地监听端口
//   - remoteHost: 远端目标主机地址
//   - remotePort: 远端目标端口
//
// 返回值:
//   - 错误信息，监听失败或转发结束时返回
//
// Example:
// ```
// // 把本地 8080 端口的流量转发到 example.com:80，此处仅作示意
// tcp.Forward(8080, "www.example.com", 80)~
// ```
func _tcpPortForward(localPort int, remoteHost string, remotePort int) error {
	return tcpServe("127.0.0.1", localPort, _tcpServeCallback(func(connection *tcpConnection) {
		log.Infof("recv local connection from: %v", connection.RemoteAddr())

		defer connection.Close()
		conn, err := _tcpConnect(remoteHost, remotePort)
		if err != nil {
			log.Errorf("connect to remote conn [%v] failed: %v", utils.HostPort(remoteHost, remotePort), err)
			return
		}
		log.Infof("create remote connection from: %v", conn.RemoteAddr())

		wg := new(sync.WaitGroup)
		wg.Add(2)
		go func() {
			defer wg.Done()
			_, _ = io.Copy(conn, connection)
		}()
		go func() {
			defer wg.Done()
			_, _ = io.Copy(connection, conn)
		}()
		wg.Wait()
	}))
}
