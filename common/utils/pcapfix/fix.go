package pcapfix

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"strconv"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/permutil"
	"github.com/yaklang/yaklang/common/utils/privileged"
)

// FixPermission 尝试修复 pcap 权限问题
// Example:
// ```
// err := pcapx.FixPermission()
// die(err) // 没有错误，即可正常使用 syn 扫描
// ...
// ```
func Fix() error {
	switch runtime.GOOS {
	case "linux":
		// setcap cap_net_raw,cap_net_admin,cap_net_bind_service+eip
		f, err := os.Executable()
		if err != nil {
			return utils.Errorf("cannot locate os.Executable: %v", err)
		}
		return permutil.Sudo(`setcap cap_net_raw,cap_net_admin,cap_net_bind_service+eip ` + strconv.Quote(f))
	case "windows":
		return utils.Error("in windows, u should just start yakit or yak.exe as administrator, or set acl for wpcap.dll")
	case "darwin":
		return fixMacOSPcap()
	}
	return nil
}

// fixMacOSPcap 修复 macOS 下的 pcap 权限
// 参考 chmod-bpf 的实现方式：
// 1. 获取系统最大 BPF 设备数
// 2. 预先创建所有 BPF 设备
// 3. 创建 access_bpf 组（如果不存在）
// 4. 将 BPF 设备的组改为 access_bpf
// 5. 设置组读写权限
// 6. 将当前用户添加到 access_bpf 组
func fixMacOSPcap() error {
	ctx := context.Background()
	executor := privileged.NewExecutor("Fix Pcap Permission")

	// 构建完整的修复脚本
	script := buildMacOSFixScript()

	log.Info("executing fix pcap permission script on macOS")
	output, err := executor.Execute(ctx, script,
		privileged.WithDescription("Fix BPF device permissions for packet capture"),
		privileged.WithTitle("Fix Pcap Permission"),
	)

	if err != nil {
		return utils.Errorf("failed to fix pcap permission: %s, output: %s", err, string(output))
	}

	log.Infof("fix pcap permission output: %s", string(output))
	return nil
}

// buildMacOSFixScript 构建 macOS 下修复 BPF 权限的脚本
// 这个脚本会：
// 1. 获取系统最大 BPF 设备数
// 2. 预先创建所有 BPF 设备（通过读取触发内核创建）
// 3. 创建 access_bpf 组（如果不存在）
// 4. 将 admin 组添加到 access_bpf 组
// 5. 将当前用户添加到 access_bpf 组
// 6. 修改所有 BPF 设备的组为 access_bpf
// 7. 设置组读写权限
func buildMacOSFixScript() string {
	const bpfGroup = "access_bpf"
	const bpfGroupName = "BPF Device ACL"
	const forceCreateBpfMax = 256

	// 获取当前用户名
	username := os.Getenv("USER")
	if username == "" {
		username = os.Getenv("LOGNAME")
	}

	script := fmt.Sprintf(`#!/bin/zsh
# BPF Permission Fix Script
# Based on chmod-bpf implementation

set -e

echo "Starting BPF permission fix..."

# 1. Get maximum number of BPF devices
SYSCTL_MAX=$(sysctl -n debug.bpf_maxdevices 2>/dev/null || echo "256")
FORCE_CREATE_BPF_MAX=%d

if [ "$FORCE_CREATE_BPF_MAX" -gt "$SYSCTL_MAX" ]; then
    FORCE_CREATE_BPF_MAX=$SYSCTL_MAX
fi

echo "Maximum BPF devices: $FORCE_CREATE_BPF_MAX"

# 2. Pre-create BPF devices by reading them
echo "Pre-creating BPF devices..."
CUR_DEV=0
while [ "$CUR_DEV" -lt "$FORCE_CREATE_BPF_MAX" ]; do
    read -r -n 0 < /dev/bpf$CUR_DEV > /dev/null 2>&1 || true
    CUR_DEV=$((CUR_DEV + 1))
done

# 3. Check if access_bpf group exists, create if not
if ! dscl . -read /Groups/%s > /dev/null 2>&1; then
    echo "Creating %s group..."
    
    # Find a free GID starting from 200
    FREE_GID=200
    while dscl . -list /Groups gid | grep -q "^%s[[:space:]]*$FREE_GID$" 2>/dev/null; do
        FREE_GID=$((FREE_GID + 1))
        if [ $FREE_GID -gt 1000 ]; then
            echo "Error: Cannot find free GID"
            exit 1
        fi
    done
    
    # Create the group
    dseditgroup -o create -n . -i $FREE_GID -r "%s" %s || true
    echo "Group %s created with GID $FREE_GID"
else
    echo "Group %s already exists"
fi

# 4. Add admin group to access_bpf group (nested group)
echo "Adding admin group to %s..."
dseditgroup -o edit -a admin -t group %s 2>/dev/null || true

# 5. Add current user to access_bpf group
if [ -n "%s" ]; then
    echo "Adding user %s to %s group..."
    dseditgroup -o edit -a %s -t user %s 2>/dev/null || true
fi

# 6. Change group ownership of all BPF devices
echo "Setting BPF device group ownership to %s..."
chgrp %s /dev/bpf* 2>/dev/null || true

# 7. Set group read/write permissions
echo "Setting BPF device group permissions..."
chmod g+rw /dev/bpf* 2>/dev/null || true

echo "BPF permission fix completed successfully!"
echo "Note: You may need to log out and log back in for group membership to take effect."
`, forceCreateBpfMax, bpfGroup, bpfGroup, bpfGroup, bpfGroupName, bpfGroup, bpfGroup, bpfGroup, bpfGroup, bpfGroup, username, username, bpfGroup, username, bpfGroup, bpfGroup, bpfGroup)

	return script
}

// WithdrawPermission 撤销 pcap 权限
// Example:
// ```
// err := pcapx.Withdraw()
// die(err)
// ...
// ```
func Withdraw() error {
	switch runtime.GOOS {
	case "linux":
		// setcap cap_net_raw,cap_net_admin,cap_net_bind_service+eip
		f, err := os.Executable()
		if err != nil {
			return utils.Errorf("cannot locate os.Executable: %v", err)
		}
		return permutil.Sudo(`setcap -r` + strconv.Quote(f))
	case "windows":
		return utils.Error("in windows, u should just start yakit or yak.exe as administrator, or set acl for wpcap.dll")
	case "darwin":
		return withdrawMacOSPcap()
	}
	return nil
}

// withdrawMacOSPcap 撤销 macOS 下的 pcap 权限
// 注意：这个函数会移除当前用户对 BPF 设备的访问权限
// 但不会删除 access_bpf 组或者从组中移除用户
func withdrawMacOSPcap() error {
	ctx := context.Background()
	executor := privileged.NewExecutor("Withdraw Pcap Permission")

	// 构建撤销权限的脚本
	script := buildMacOSWithdrawScript()

	log.Info("executing withdraw pcap permission script on macOS")
	output, err := executor.Execute(ctx, script,
		privileged.WithDescription("Remove BPF device permissions"),
		privileged.WithTitle("Withdraw Pcap Permission"),
	)

	if err != nil {
		return utils.Errorf("failed to withdraw pcap permission: %s, output: %s", err, string(output))
	}

	log.Infof("withdraw pcap permission output: %s", string(output))
	return nil
}

// buildMacOSWithdrawScript 构建撤销 BPF 权限的脚本
// 这个脚本会移除 BPF 设备的组读写权限
func buildMacOSWithdrawScript() string {
	script := `#!/bin/zsh
# BPF Permission Withdraw Script

set -e

echo "Starting BPF permission withdrawal..."

# Remove group read/write permissions from all BPF devices
echo "Removing BPF device group permissions..."
chmod g-rw /dev/bpf* 2>/dev/null || true

# Optionally, you can also reset the group to wheel (default)
# chgrp wheel /dev/bpf* 2>/dev/null || true

echo "BPF permission withdrawal completed successfully!"
`

	return script
}
