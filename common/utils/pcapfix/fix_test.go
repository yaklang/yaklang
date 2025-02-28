package pcapfix

import (
	"testing"
)

func TestFix(t *testing.T) {
	//lookupBpf := func() {
	//	output, err := exec.Command("sh", "-c", "ls -al /dev/bpf*").Output()
	//	require.NoError(t, err)
	//	fmt.Println(string(output))
	//}
	//
	//lookupBpf()
	Fix()
	//lookupBpf()
}
