package yaklib

import (
	"io"
	"sync"
	"yaklang/common/log"
	"yaklang/common/utils"
)

type portForward struct {
}

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
