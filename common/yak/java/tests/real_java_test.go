package tests

import (
	_ "embed"
	"runtime"
	"testing"
	"time"

	"net/http"
	_ "net/http/pprof"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssa/ssaprofile"
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
	t.Skip()

	go func() {
		log.Println(http.ListenAndServe("localhost:18080", nil)) // 启动 pprof 服务
	}()

	runtime.SetBlockProfileRate(1)

	// path := `/Users/wlz/Developer/Target/yakssaExample/java-sec-code`
	path := `/Users/wlz/Developer/Target/yakssaExample/spring-boot`

	// relfs := filesys.NewRelLocalFs(path)
	// filesys.Recursive(
	// 	".", filesys.WithFileSystem(relfs),
	// 	filesys.WithFileStat(func(s string, fi fs.FileInfo) error {
	// 		if fi.IsDir() {
	// 			return nil
	// 		}
	// 		if relfs.Ext(s) == ".java" {
	// 			data, err := relfs.ReadFile(s)
	// 			require.NoError(t, err)
	// 			java2ssa.Frontend(string(data), false)
	// 		}
	// 		return nil
	// 	}),
	// )
	log.SetLevel(log.DebugLevel)
	progName := uuid.NewString()
	_ = progName
	start := time.Now()
	prog, err := ssaapi.ParseProject(
		ssaapi.WithFileSystem(filesys.NewRelLocalFs(path)),
		ssaapi.WithLanguage(ssaapi.JAVA),
		ssaapi.WithProgramName(progName),
		ssaapi.WithProcess(func(msg string, process float64) {
			log.Errorf("DB--Process: %s, %.2f%%", msg, process*100)
		}),
	)
	_ = prog
	databaseTime := time.Since(start)
	// ssa.ShowDatabaseCacheCost()
	defer ssadb.DeleteProgram(ssadb.GetDB(), progName)
	require.NoError(t, err)

	databaseCost := ssaprofile.GetProfileListMap()
	_ = databaseCost

	start = time.Now()
	if false {
		// memory
		ssaprofile.Refresh()
		_, err := ssaapi.ParseProject(
			ssaapi.WithFileSystem(filesys.NewRelLocalFs(path)),
			ssaapi.WithLanguage(ssaapi.JAVA),
			ssaapi.WithProcess(func(msg string, process float64) {
				log.Errorf("Mem-Process: %s, %.2f%%", msg, process*100)
			}),
		)
		// ssa.ShowDatabaseCacheCost()
		require.NoError(t, err)
	}
	memoryTime := time.Since(start)
	memoryCost := ssaprofile.GetProfileListMap()

	if true {
		ssaprofile.ShowCacheCost(memoryCost)
		log.Errorf("----------------------------------------------------------------------------------------------")
		log.Errorf("----------------------------------------------------------------------------------------------")
		log.Errorf("----------------------------------------------------------------------------------------------")
		log.Errorf("----------------------------------------------------------------------------------------------")
		ssaprofile.ShowCacheCost(databaseCost)
		log.Errorf("----------------------------------------------------------------------------------------------")
		log.Errorf("----------------------------------------------------------------------------------------------")
		log.Errorf("----------------------------------------------------------------------------------------------")
		log.Errorf("----------------------------------------------------------------------------------------------")
		ssaprofile.ShowDiffCacheCost(databaseCost, memoryCost)
		// _ = prog
		log.Errorf("----------------------------------------------------------------------------------------------")
		log.Errorf("----------------------------------------------------------------------------------------------")
		log.Errorf("Database time: %s, Memory time: %s", databaseTime, memoryTime)
		log.Errorf("----------------------------------------------------------------------------------------------")
		log.Errorf("----------------------------------------------------------------------------------------------")
	}
}
