package pcapfix

import (
	"fmt"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"
	"os/exec"
	"runtime"
	"testing"
)

func TestFix(t *testing.T) {
	Fix()
}

func TestFixMacos(t *testing.T) {
	if utils.InGithubActions() {
		t.Skip("Skip in Github Actions")
	}
	if runtime.GOOS != "darwin" {
		t.Skip("Skip on non-darwin")
	}
	lookupBpf := func() {
		output, err := exec.Command("sh", "-c", "ls -al /dev/bpf*").Output()
		require.NoError(t, err)
		fmt.Println(string(output))
	}
	lookupBpf()
	require.NoError(t, Fix())
	lookupBpf()
	require.True(t, IsPrivilegedForNetRaw())
	require.NoError(t, Withdraw())
	lookupBpf()
	require.False(t, IsPrivilegedForNetRaw())

}
