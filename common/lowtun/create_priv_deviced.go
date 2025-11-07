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

func CreatePrivilegedDevice(mtu int) (Device, error) {
	var socketName string
	if utils.IsWindows() {
		socketName = "lowtun.pipe"
	} else {
		socketName = "lowtun.sock"
	}
	var socketPath = filepath.Join(consts.GetDefaultYakitBaseTempDir(), socketName)

	tDev, err := CreateDeviceFromSocket(socketPath, mtu)
	if !utils.IsNil(tDev) {
		return tDev, nil
	}

	currentBinary, err := os.Executable()
	if err != nil {
		return nil, utils.Errorf("Failed to get current binary path: %v", err)
	}

	prepared := exec.Command(currentBinary, "forward-tun-to-socks", "-h")
	var out bytes.Buffer
	prepared.Stdout = &out
	prepared.Stderr = &out
	err = prepared.Run()
	if err != nil {
		return nil, utils.Errorf("Failed to prepare privileged executor: %v, output: %s", err, out.String())
	}
	if !strings.Contains(out.String(), `Create a TUN device and forward traffic`) {
		return nil, utils.Errorf("Failed to check `forward-tun-to-socks`, output: %s, check flag 'Create a TUN device and forward traffic'", out.String())
	}

	errChan := make(chan error)

	go func() {
		executor := privileged.NewExecutor("CreateLowTunDevice")
		outBytes, err := executor.Execute(
			context.Background(),
			fmt.Sprintf(
				"%v forward-tun-to-socks --socket-path %#v",
				currentBinary, socketPath,
			),
		)
		if err != nil {
			errChan <- err
			log.Errorf("Failed to create lowtun device: %v, output: %s", err, string(outBytes))
		} else {
			errChan <- nil
		}
		_ = outBytes
	}()

	start := time.Now()
	for {
		select {
		case err, ok := <-errChan:
			if ok {
				if err != nil {
					return nil, err
				}
			}
		case <-time.After(time.Millisecond * 500):
			// check existed
			tDev, err := CreateDeviceFromSocket(socketPath, mtu)
			if err != nil {
				if time.Since(start) > time.Second*10 {
					return nil, utils.Errorf("Timeout waiting for privileged device creation: %v", err)
				}
				continue
			}
			if !utils.IsNil(tDev) {
				return tDev, nil
			}
		}
	}
}
