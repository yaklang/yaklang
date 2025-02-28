package pcapfix

import (
	"context"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/permutil"
	"github.com/yaklang/yaklang/common/utils/privileged"
	"os"
	"runtime"
	"strconv"
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

		cmd := "chmod +rw /dev/bpf*"
		output, err := privileged.NewExecutor("fix pcap").Execute(context.Background(), cmd, privileged.WithDescription("fix pcap permission for user"))
		if err != nil {
			return utils.Errorf("cannot create group access_bpf: %s ,output: %s", err, output)
		}
		return nil
	}
	return nil
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
		cmd := "chmod -rw /dev/bpf*"
		output, err := privileged.NewExecutor("withdraw pcap").Execute(context.Background(), cmd, privileged.WithDescription("withdraw pcap permission for user"))
		if err != nil {
			return utils.Errorf("cannot create group access_bpf: %s ,output: %s", err, output)
		}
		return nil
	}
	return nil
}
