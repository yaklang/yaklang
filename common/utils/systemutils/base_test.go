package systemutils

import (
	"fmt"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"
	"os/exec"
	"testing"
)

func TestChmodBpf(t *testing.T) {
	if utils.InGithubActions() {
		t.Skip("Skip in Github Actions")
	}
	lookupBpf := func() {
		output, err := exec.Command("sh", "-c", "ls -al /dev/bpf*").Output()
		require.NoError(t, err)
		fmt.Println(string(output))
	}

	lookupBpf()
	err := ChmodBpfSet()
	require.NoError(t, err)
	lookupBpf()
	err = ChmodBbfUnset()
	require.NoError(t, err)
	lookupBpf()
}
