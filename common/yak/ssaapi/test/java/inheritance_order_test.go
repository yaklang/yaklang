package java

import (
	"fmt"
	"sort"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

// Class fields can instantiate an implementation before its lazy inheritance
// declaration is visited. Every declaration order must index those instances
// under the interface too, so IA.set finds all three call sites.
func TestJavaHierarchyBeforeFieldInitializers(t *testing.T) {
	sources := map[string]string{
		"IA":     `package p; interface IA { void set(int value); }`,
		"First":  `package p; class First implements IA { void set(int value) {} }`,
		"Second": `package p; class Second implements IA { void set(int value) {} }`,
		"Consumer": `package p;
class Consumer {
    IA face;
    First first;
    Second second;
    void run() { face.set(0); first.set(1); second.set(2); }
}`,
	}
	for _, order := range [][]string{
		{"Consumer", "IA", "First", "Second"},
		{"IA", "Consumer", "First", "Second"},
		{"IA", "First", "Consumer", "Second"},
		{"IA", "First", "Second", "Consumer"},
	} {
		t.Run(fmt.Sprint(order), func(t *testing.T) {
			vf := filesys.NewVirtualFs()
			for index, name := range order {
				vf.AddFile(fmt.Sprintf("p/%02d_%s.java", index, name), sources[name])
			}
			programs, err := ssaapi.ParseProjectWithFS(vf,
				ssaapi.WithLanguage(ssaconfig.JAVA), ssaapi.WithMemory(),
				ssaconfig.WithCompileConcurrency(4))
			require.NoError(t, err)
			result, err := programs.SyntaxFlowWithError(`
IA as $instances
IA.set(* #-> ?{opcode: const} as $value) as $call`)
			require.NoError(t, err)
			var got []string
			for _, value := range result.GetValues("value") {
				got = append(got, value.String())
			}
			sort.Strings(got)
			t.Logf("interface instances: %v", result.GetValues("instances"))
			require.Equal(t, []string{"0", "1", "2"}, got)
			require.Len(t, result.GetValues("call"), 3)
		})
	}
}
