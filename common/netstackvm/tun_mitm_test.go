package netstackvm_test

import (
	"bytes"
	"context"
	"fmt"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/netstackvm"
	"github.com/yaklang/yaklang/common/yak/yaklib"
	"testing"
)

func TestMitmTun(t *testing.T) {
	vm, err := netstackvm.NewSystemNetStackVM()
	require.NoError(t, err)
	server, err := yaklib.NewMITMServer(
		yaklib.MITMConfigDialer(vm.DialTCP),
		yaklib.MITMConfigHijackHTTPResponse(func(isHttps bool, u string, rsp []byte, modified func([]byte), dropped func()) {
			modified(bytes.Replace(rsp, []byte("百度"), []byte("yak"), -1))
		}),
	)
	require.NoError(t, err)
	ctx := context.Background()
	tundev, err := netstackvm.NewTunVirtualMachine(ctx)
	require.NoError(t, err)
	defer tundev.Close()

	err = tundev.HijackDomain("www.baidu.com")
	require.NoError(t, err)

	err = server.ServerListener(ctx, tundev.GetListener())
	if err != nil {
		fmt.Println(err)
		return
	}
}
