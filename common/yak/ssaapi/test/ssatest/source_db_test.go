package ssatest

import (
	"fmt"
	"io/fs"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

func checkSource(vf filesys_interface.FileSystem, t *testing.T, opt ...ssaconfig.Option) {
	progName := uuid.NewString()
	opt = append(opt,
		ssaapi.WithProgramName(progName),
	)
	prog, err := ssaapi.ParseProjectWithFS(vf, opt...)
	require.NoError(t, err)
	require.NotNil(t, prog)
	defer ssadb.DeleteProgram(ssadb.GetDB(), progName)

	tmp := make(map[string]struct{})
	irfs := ssadb.NewIrSourceFs()
	filesys.Recursive(fmt.Sprintf("/%s", progName),
		filesys.WithFileSystem(irfs),
		filesys.WithFileStat(func(s string, fi fs.FileInfo) error {
			log.Info(s)
			if _, ok := tmp[s]; ok {
				t.Fatalf("file %s already exists", s)
			}
			tmp[s] = struct{}{}
			return nil
		}),
	)
	log.Infof("file: %v", tmp)
	require.Equal(t, 3, len(tmp))
}

func TestSourceWithInclude_JaaAVA(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("src/main/java/A.java", `
	package A; 
	class A {
		public  int get() {
			return 	 1;
		}
	}
	`)
	vf.AddFile("src/main/java/B.java", `
	package B; 
	import A.A;
	class B {
		public static void main(String[] args) {
			A a = new A();
			println(a.get());
		}
	}
	`)
	vf.AddFile("src/main/java/C.java", `
	package C; 
	import A.A;
	class C {
	}
	`)
	checkSource(vf, t, ssaapi.WithLanguage(ssaconfig.JAVA))
}

func TestSourceWithInclude_PHP(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("a.php", `<?php
	include 'b.php';
	`)
	vf.AddFile("b.php", `<?php
	echo "hello";`)
	vf.AddFile("c.php", `<?php
	include 'b.php';`)
	checkSource(vf, t)
}

func TestTypeSaveLoad(t *testing.T) {
	code := `
	a = int(1)
	`
	CheckSyntaxFlow(t, code,
		`a as $t `, map[string][]string{
			"t": {"castType(number, 1)"},
		},
		ssaapi.WithLanguage(ssaconfig.Yak),
	)
}

func TestMarshalInstruction(t *testing.T) {
	t.Run("type cast", func(t *testing.T) {
		code := `
		<?php
		$a = (int) 1;
		println($a);
		`
		CheckSyntaxFlow(t, code, `println(* as $para); $para #-> as $top`, map[string][]string{
			"para": {"castType(number, 1)"},
			"top":  {"1"},
		}, ssaapi.WithLanguage(ssaconfig.PHP))
	})
}
