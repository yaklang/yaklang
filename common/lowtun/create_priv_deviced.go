package lowtun

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/privileged"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

const (
	// 用于存储高权限进程密码的 key（使用固定的 MD5 值）
	privilegedProcessSecretKey = "yaklang_tun_privileged_secret_a3f8c9d2e1b7"
)

// getOrCreatePrivilegedSecret 获取或创建高权限进程的密码
// 这个密码会被持久化存储在数据库中，保证同一台机器上的多个进程可以复用
func getOrCreatePrivilegedSecret() string {
	db := consts.GetGormProfileDatabase()
	if db == nil {
		// 如果数据库不可用，生成临时密码
		hash := md5.Sum([]byte(fmt.Sprintf("%d", time.Now().UnixNano())))
		return hex.EncodeToString(hash[:])
	}

	// 尝试从数据库获取已存储的密码
	secret := yakit.GetKey(db, privilegedProcessSecretKey)
	if secret != "" {
		log.Infof("using existing privileged secret from database")
		return secret
	}

	// 生成新密码并存储
	hash := md5.Sum([]byte(fmt.Sprintf("%d", time.Now().UnixNano())))
	secret = hex.EncodeToString(hash[:])

	err := yakit.SetKey(db, privilegedProcessSecretKey, secret)
	if err != nil {
		log.Errorf("failed to store privileged secret: %v", err)
	} else {
		log.Infof("generated and stored new privileged secret")
	}

	return secret
}

// ResetPrivilegedSecret 重置高权限进程的密码
// 这会生成一个新的密码并存储到数据库中，导致旧的高权限进程无法被复用
func ResetPrivilegedSecret() (string, error) {
	db := consts.GetGormProfileDatabase()
	if db == nil {
		return "", utils.Errorf("database not available")
	}

	// 生成新密码
	hash := md5.Sum([]byte(fmt.Sprintf("%d", time.Now().UnixNano())))
	newSecret := hex.EncodeToString(hash[:])

	// 存储到数据库
	err := yakit.SetKey(db, privilegedProcessSecretKey, newSecret)
	if err != nil {
		return "", utils.Errorf("failed to store new privileged secret: %v", err)
	}

	log.Infof("successfully reset privileged secret: %s", newSecret)
	return newSecret, nil
}

// killProcessPrivileged 使用高权限方式 kill 指定的进程
// 返回 true 表示用户确认 kill，false 表示用户取消
func killProcessPrivileged(pid int, socketPath string) (bool, error) {
	log.Infof("attempting to kill privileged process with PID %d", pid)

	executor := privileged.NewExecutor("KillPrivilegedProcess")
	_, err := executor.Execute(
		context.Background(),
		fmt.Sprintf("kill -9 %d", pid),
		privileged.WithTitle("Kill Privileged TUN Process"),
		privileged.WithDescription(fmt.Sprintf("Kill the privileged process (PID: %d) that is blocking socket: %s", pid, socketPath)),
	)

	if err != nil {
		// 检查是否是用户取消
		if strings.Contains(err.Error(), "User canceled") ||
			strings.Contains(err.Error(), "user cancelled") ||
			strings.Contains(err.Error(), "cancelled") {
			log.Infof("user cancelled kill operation")
			return false, nil
		}
		log.Errorf("failed to kill process with privilege: %v", err)
		return false, err
	}

	log.Infof("successfully killed process %d", pid)
	return true, nil
}

// readPIDFromLockFile 从 PID lock 文件读取 PID
func readPIDFromLockFile(lockPath string) (int, error) {
	data, err := os.ReadFile(lockPath)
	if err != nil {
		return 0, err
	}

	pidStr := strings.TrimSpace(string(data))
	pid, err := strconv.Atoi(pidStr)
	if err != nil {
		return 0, utils.Errorf("invalid PID in lock file: %s", pidStr)
	}

	return pid, nil
}

const (
	tunnelPrivilegedSocketPrefix = "lowtun"
	routePrivilegedSocketPrefix  = "route"

	tunnelPrivilegedCmd = "forward-tun-to-sock"
	routePrivilegedCmd  = "modify-route-to-socks"
)

func CreatePrivilegedDevice(mtu int) (Device, string, error) {
	return CreatePrivilegedDeviceEx(mtu, tunnelPrivilegedSocketPrefix, tunnelPrivilegedCmd)
}

func CreatePrivilegedRouteDevice(mtu int) (Device, string, error) {
	return CreatePrivilegedDeviceEx(mtu, routePrivilegedSocketPrefix, routePrivilegedCmd)
}

func CreatePrivilegedDeviceEx(mtu int, socketPrefix string, cmd string) (Device, string, error) {
	var socketName string
	if utils.IsWindows() {
		socketName = ".pipe"
	} else {
		socketName = ".sock"
	}
	socketName = socketPrefix + socketName

	var socketPath = filepath.Join(consts.GetDefaultYakitBaseTempDir(), socketName)

	log.Infof("attempting to create privileged device with socket path: %s", socketPath)

	// 获取或创建固定的密码（从数据库持久化存储）
	secret := getOrCreatePrivilegedSecret()

	// 首先尝试用当前密码连接已有的高权限进程
	tDev, tunName, err := CreateDeviceFromSocket(socketPath, mtu, secret)
	if !utils.IsNil(tDev) && err == nil {
		log.Infof("successfully reused existing privileged process: socket=%s, utun=%s", socketPath, tunName)
		return tDev, tunName, nil
	}

	// 如果 socket 文件存在，需要判断具体情况
	pidLockPath := socketPath + ".pid.lock"
	if _, statErr := os.Stat(socketPath); statErr == nil {
		// 检查是否是 connection refused（说明没有进程监听）
		if err != nil && (strings.Contains(err.Error(), "connection refused") ||
			strings.Contains(err.Error(), "connect: no such file")) {
			// socket 文件存在但没有进程监听，删除旧的 socket 文件
			log.Warnf("socket file exists but no process is listening, removing stale socket: %s", socketPath)
			if removeErr := os.Remove(socketPath); removeErr != nil {
				log.Errorf("failed to remove stale socket: %v", removeErr)
				return nil, "", utils.Errorf("failed to remove stale socket %s: %v", socketPath, removeErr)
			}
			// 同时删除 PID lock 文件
			os.Remove(pidLockPath)
			log.Infof("removed stale socket and PID lock, will create new privileged process")
		} else if err != nil && strings.Contains(err.Error(), "authentication failed") {
			// 真正的认证失败：能连接但密码错误
			log.Errorf("socket exists and process is running, but authentication failed")
			log.Errorf("this means the privileged process is using a different secret")

			// 尝试从 PID lock 文件读取 PID
			if pid, readErr := readPIDFromLockFile(pidLockPath); readErr == nil {
				log.Infof("found PID lock file with PID: %d", pid)
				log.Infof("attempting to kill the privileged process with user confirmation...")

				// 使用 privileged 方式 kill 进程（会弹出用户确认对话框）
				confirmed, killErr := killProcessPrivileged(pid, socketPath)
				if !confirmed {
					// 用户取消了 kill 操作
					log.Errorf("user cancelled the kill operation, cannot proceed")
					return nil, "", utils.Errorf("authentication failed and user cancelled killing the existing process (PID: %d)\n"+
						"Cannot start new privileged process while old one is running.\n"+
						"Socket: %s", pid, socketPath)
				}

				if killErr != nil {
					log.Errorf("failed to kill process: %v", killErr)
					return nil, "", utils.Errorf("failed to kill existing privileged process (PID: %d): %v\n"+
						"Please manually kill the process and try again.", pid, killErr)
				}

				// Kill 成功，删除 socket 和 PID lock 文件
				log.Infof("successfully killed process %d, cleaning up files...", pid)
				os.Remove(socketPath)
				os.Remove(pidLockPath)
				log.Infof("cleaned up socket and PID lock files, will create new privileged process")
			} else {
				// 没有 PID lock 文件或读取失败
				log.Warnf("cannot read PID lock file: %v", readErr)
				log.Errorf("authentication failed but cannot determine process PID")
				return nil, "", utils.Errorf("cannot authenticate to existing privileged process at %s: %v\n\n"+
					"A privileged process is running but using a different password.\n"+
					"Cannot determine the process PID (no PID lock file found).\n"+
					"Please manually kill the existing process and try again.\n"+
					"You can find the process with: ps aux | grep '%s'\n"+
					"Then kill it with: sudo kill -9 <PID>", socketPath, err, cmd)
			}
		} else if err != nil {
			// 其他未知错误
			log.Warnf("socket exists but connection failed with error: %v", err)
			log.Warnf("will try to remove socket and create new process")
			if removeErr := os.Remove(socketPath); removeErr != nil {
				log.Errorf("failed to remove socket: %v", removeErr)
			}
			os.Remove(pidLockPath)
		}
	} else {
		log.Infof("no existing socket found, will start new privileged process")
	}

	currentBinary, err := os.Executable()
	if err != nil {
		return nil, "", utils.Errorf("Failed to get current binary path: %v", err)
	}

	log.Infof("checking binary capability: %s %s", currentBinary, cmd)

	prepared := exec.Command(currentBinary, cmd, "-h")
	var out bytes.Buffer
	prepared.Stdout = &out
	prepared.Stderr = &out
	err = prepared.Run()
	if err != nil {
		return nil, "", utils.Errorf("Failed to prepare privileged executor: %v, output: %s", err, out.String())
	}
	if !strings.Contains(out.String(), `Unix socket path`) {
		return nil, "", utils.Errorf("Failed to check `%s`, output: %s, check flag 'Unix socket path'", out.String(), cmd)
	}

	log.Infof("binary capability check passed, starting privileged executor")

	// 用于记录进程启动的标志
	processStarted := make(chan struct{})
	errChan := make(chan error, 1)

	go func() {
		log.Infof("starting privileged executor goroutine")
		executor := privileged.NewExecutor("CreatePrivilegedDevice")
		log.Infof("executing privileged command: %s %s --socket-path %s --secret %s", currentBinary, socketPath, secret, cmd)
		_, err := executor.Execute(
			context.Background(),
			fmt.Sprintf(
				"%v %s --socket-path %#v --secret %s",
				currentBinary, cmd, socketPath, secret,
			),
			privileged.WithSkipConfirmDialog(),
			privileged.WithTitle("CreatePrivilegedDevice"),
			privileged.WithDescription(fmt.Sprintf("Create a Privileged device and forward traffic or modify route: %v", socketPath)),
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
			tDev, tunName, err := CreateDeviceFromSocket(socketPath, mtu, secret)
			if err == nil && !utils.IsNil(tDev) {
				log.Infof("successfully connected to privileged device via socket: %s, utun: %s", socketPath, tunName)
				return tDev, tunName, nil
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
