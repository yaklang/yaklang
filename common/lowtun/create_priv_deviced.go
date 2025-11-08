package lowtun

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/privileged"
)

func CreatePrivilegedDevice(mtu int) (Device, string, error) {
	var socketName string
	if utils.IsWindows() {
		socketName = "lowtun.pipe"
	} else {
		socketName = "lowtun.sock"
	}
	var socketPath = filepath.Join(consts.GetDefaultYakitBaseTempDir(), socketName)

	log.Infof("attempting to create privileged device with socket path: %s", socketPath)

	tDev, _ := CreateDeviceFromSocket(socketPath, mtu)
	if !utils.IsNil(tDev) {
		log.Infof("found existing device from socket: %s", socketPath)
		return tDev, socketPath, nil
	}

	log.Infof("no existing device found, preparing privileged executor")

	currentBinary, err := os.Executable()
	if err != nil {
		return nil, "", utils.Errorf("Failed to get current binary path: %v", err)
	}

	log.Infof("checking binary capability: %s forward-tun-to-socks", currentBinary)

	prepared := exec.Command(currentBinary, "forward-tun-to-socks", "-h")
	var out bytes.Buffer
	prepared.Stdout = &out
	prepared.Stderr = &out
	err = prepared.Run()
	if err != nil {
		return nil, "", utils.Errorf("Failed to prepare privileged executor: %v, output: %s", err, out.String())
	}
	if !strings.Contains(out.String(), `Create a TUN device and forward traffic`) {
		return nil, "", utils.Errorf("Failed to check `forward-tun-to-socks`, output: %s, check flag 'Create a TUN device and forward traffic'", out.String())
	}

	log.Infof("binary capability check passed, starting privileged executor")

	// 用于记录进程启动的标志
	processStarted := make(chan struct{})
	errChan := make(chan error, 1)

	go func() {
		log.Infof("starting privileged executor goroutine")
		executor := privileged.NewExecutor("CreateLowTunDevice")
		log.Infof("executing privileged command: %s forward-tun-to-socks --socket-path %s", currentBinary, socketPath)
		_, err := executor.Execute(
			context.Background(),
			fmt.Sprintf(
				"%v forward-tun-to-socks --socket-path %#v",
				currentBinary, socketPath,
			),
			privileged.WithSkipConfirmDialog(),
			privileged.WithTitle("CreateHijackTUNDevice"),
			privileged.WithDescription(fmt.Sprintf("Create a TUN device and forward traffic to unix socket: %v", socketPath)),
			privileged.WithDiscardStdoutAndStderr(),
			privileged.WithBeforePrivilegedProcessExecute(func() {
				log.Infof("privileged process is starting, notifying main goroutine")
				close(processStarted)
			}),
		)
		if err != nil {
			log.Errorf("privileged executor failed: %v", err)
			errChan <- err
		} else {
			log.Infof("privileged executor completed successfully")
		}
	}()

	log.Infof("waiting for privileged process to start")
	select {
	case err := <-errChan:
		log.Errorf("received error before process started: %v", err)
		return nil, "", err
	case <-processStarted:
		log.Infof("privileged process started, polling for socket creation")
		// 进程已启动，开始轮询检查 socket 是否创建成功
		start := time.Now()
		for {
			// 先检查是否有错误
			select {
			case err := <-errChan:
				log.Errorf("received error from privileged executor: %v", err)
				return nil, "", err
			default:
			}

			// 尝试连接 socket
			tDev, err := CreateDeviceFromSocket(socketPath, mtu)
			if err == nil && !utils.IsNil(tDev) {
				log.Infof("successfully connected to privileged device via socket: %s", socketPath)
				return tDev, socketPath, nil
			}

			// 检查超时
			elapsed := time.Since(start)
			if elapsed > 60*time.Second {
				log.Errorf("timeout waiting for socket creation after %v", elapsed)
				return nil, "", utils.Errorf("timeout waiting for privileged device creation: socket not available after 10s")
			}

			log.Infof("socket not ready yet (elapsed: %v), retrying in 500ms...", elapsed)
			time.Sleep(time.Second)
		}
	}
}
