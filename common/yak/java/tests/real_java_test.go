package tests

import (
	_ "embed"
	"fmt"
	"runtime"
	"testing"
	"time"

	_ "net/http/pprof"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

//go:embed code/DynamicSecurityMetadataSource.java
var DynamicSecurityMetadataSource string

func TestRealJava_PanicInMemberCall(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("DynamicSecurityMetadataSource.java", DynamicSecurityMetadataSource)
	ssatest.CheckWithFS(vf, t, func(prog ssaapi.Programs) error {
		return nil
	}, ssaapi.WithLanguage(ssaapi.JAVA))
}

func TestA(t *testing.T) {

	runtime.SetBlockProfileRate(1)

	path := `/Users/wlz/Developer/Target/yakssaExample/java-sec-code`

	log.SetLevel(log.ErrorLevel)
	progName := uuid.NewString()
	_ = progName
	start := time.Now()
	prog, err := ssaapi.ParseProject(
		ssaapi.WithFileSystem(filesys.NewRelLocalFs(path)),
		ssaapi.WithLanguage(ssaapi.JAVA),
		ssaapi.WithProgramName(progName),
		ssaapi.WithProcess(func(msg string, process float64) {
			log.Errorf("Process: %s, %.2f%%", msg, process*100)
		}),
	)
	log.Errorf("ParseProject cost: %v", time.Since(start))
	ssa.ShowDatabaseCacheCost()
	defer ssadb.DeleteProgram(ssadb.GetDB(), progName)

	require.NoError(t, err)
	_ = prog

	var a string
	fmt.Scan("%s", &a)

}
