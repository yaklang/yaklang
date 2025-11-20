package tests

import (
	"context"
	_ "embed"
	"os"
	"path/filepath"
	"runtime"
	"runtime/trace"
	"testing"
	"time"

	"net/http"
	_ "net/http/pprof"

	"github.com/google/uuid"
	"github.com/jinzhu/gorm"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/diagnostics"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssa/ssalog"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssareducer"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

//go:embed code/DynamicSecurityMetadataSource.java
var DynamicSecurityMetadataSource string

func TestRealJava_PanicInMemberCall(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("DynamicSecurityMetadataSource.java", DynamicSecurityMetadataSource)
	ssatest.CheckWithFS(vf, t, func(prog ssaapi.Programs) error {
		return nil
	}, ssaapi.WithLanguage(ssaconfig.JAVA))
}

func TestCodeCompile(t *testing.T) {
	t.Skip()

	go func() {
		log.Println(http.ListenAndServe("localhost:18080", nil)) // 启动 pprof 服务
	}()
	_ = ssadb.GetDB()

	runtime.SetBlockProfileRate(1)

	path := `/Users/wlz/Developer/Target/yakssaExample/java-sec-code`
	// path := `/Users/wlz/Developer/Target/yakssaExample/spring-boot`

	db, err := gorm.Open(consts.SQLiteExtend, "file::memory:?cache=shared")
	if err != nil {
		log.Errorf("failed to open gorm database: %v", err)
		panic(utils.Errorf("failt open memory database "))
		// return
	}
	// db = db.Debug()
	consts.SetGormSSAProjectDatabase(db)
	schema.AutoMigrate(db, schema.KEY_SCHEMA_SSA_DATABASE)
	// reference: https://stackoverflow.com/questions/35804884/sqlite-concurrent-writing-performance
	db.DB().SetConnMaxLifetime(time.Hour)
	db.DB().SetMaxIdleConns(10)
	// set MaxOpenConns to disable connections pool, for write speed and "database is locked" error
	db.DB().SetMaxOpenConns(1)

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
	// 			java2ssa.Frontend(string(data))
	// 		}
	// 		return nil
	// 	}),
	// )
	log.SetLevel(log.DebugLevel)
	ssalog.Log.SetLevel("debug")
	progName := uuid.NewString()
	_ = progName
	start := time.Now()
	prog, err := ssaapi.ParseProject(
		ssaapi.WithFileSystem(filesys.NewRelLocalFs(path)),
		ssaapi.WithLanguage(ssaconfig.JAVA),
		ssaapi.WithProgramName(progName),
		ssaapi.WithProcess(func(msg string, process float64) {
			log.Errorf("DB--Process: %s, %.2f%%", msg, process*100)
		}),
	)
	_ = prog
	databaseTime := time.Since(start)
	// diagnostics.LogCompileSummary()
	defer ssadb.DeleteProgram(ssadb.GetDB(), progName)
	require.NoError(t, err)

	databaseRecorder := diagnostics.DefaultRecorder()
	memoryRecorder := databaseRecorder

	start = time.Now()
	if false {
		// memory
		memoryRecorder = diagnostics.ResetDefaultRecorder()
		_, err := ssaapi.ParseProject(
			ssaapi.WithFileSystem(filesys.NewRelLocalFs(path)),
			ssaapi.WithLanguage(ssaconfig.JAVA),
			ssaapi.WithProcess(func(msg string, process float64) {
				log.Errorf("Mem-Process: %s, %.2f%%", msg, process*100)
			}),
		)
		// diagnostics.LogCompileSummary()
		require.NoError(t, err)
	}
	memoryTime := time.Since(start)
	if memoryRecorder == diagnostics.DefaultRecorder() {
		memoryRecorder = diagnostics.DefaultRecorder()
	}

	if true {
		diagnostics.LogRecorder("memory", memoryRecorder)
		log.Errorf("----------------------------------------------------------------------------------------------")
		log.Errorf("----------------------------------------------------------------------------------------------")
		log.Errorf("----------------------------------------------------------------------------------------------")
		log.Errorf("----------------------------------------------------------------------------------------------")
		diagnostics.LogRecorder("database", databaseRecorder)
		log.Errorf("----------------------------------------------------------------------------------------------")
		log.Errorf("----------------------------------------------------------------------------------------------")
		log.Errorf("----------------------------------------------------------------------------------------------")
		log.Errorf("----------------------------------------------------------------------------------------------")
		diagnostics.CompareRecorderCosts(databaseRecorder, memoryRecorder)
		// _ = prog
		log.Errorf("----------------------------------------------------------------------------------------------")
		log.Errorf("----------------------------------------------------------------------------------------------")
		log.Errorf("Database time: %s, Memory time: %s", databaseTime, memoryTime)
		log.Errorf("----------------------------------------------------------------------------------------------")
		log.Errorf("----------------------------------------------------------------------------------------------")
	}
	// select {}
}

func TestCodeScan(t *testing.T) {
	t.Skip()

	f, err := os.Create("trace.out")
	if err != nil {
		log.Fatal(err)
		return
	}
	path, err := filepath.Abs(f.Name())
	log.Infof("path: %s, %s, %v", f.Name(), path, err)
	defer f.Close()

	err = trace.Start(f)
	if err != nil {
		log.Fatal(err)
		return
	}
	defer trace.Stop()

	vf := filesys.NewVirtualFs()
	code := `
package com.example.demo.controller.deepcross;

import com.example.demo.controller.utils.DummyUtil;
import org.springframework.http.HttpStatus;
import org.springframework.http.ResponseEntity;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.RequestParam;
import org.springframework.web.bind.annotation.RestController;

@RestController
public class DeepCrossController {
    @GetMapping({"/xss/direct/1", "/xss/no-cross/1"})
    public ResponseEntity<String> noDeepCross(@RequestParam(required = false) String body) {
        if (body == null) {
            return ResponseEntity.ok("No input, try <a href='/xss/no-cross?body=hello-world'>here</a>");
        }
        ResponseEntity<String> resp = ResponseEntity.ok(body);
        return resp;
    }

    @GetMapping({"/xss/direct/2", "/xss/no-cross/2"})
    public ResponseEntity<String> noDeepCross1(@RequestParam(required = false) String body) {
        if (body == null) {
            return ResponseEntity.ok("No input, try <a href='/xss/no-cross?body=hello-world'>here</a>");
        }
        ResponseEntity<String> resp = ResponseEntity.ok().body(body);
        return resp;
    }

    @GetMapping({"/xss/direct/3", "/xss/no-cross/3"})
    public ResponseEntity<String> noDeepCross2(@RequestParam(required = false) String body) {
        if (body == null) {
            return ResponseEntity.ok("No input, try <a href='/xss/no-cross?body=hello-world'>here</a>");
        }
        ResponseEntity<String> resp = new ResponseEntity(body, HttpStatus.OK);
        return resp;
    }

    @GetMapping({"/xss/direct/4", "/xss/no-cross/4"})
    public ResponseEntity<String> noDeepCross4(@RequestParam(required = false) String body) {
        if (body == null) {
            return ResponseEntity.ok("No input, try <a href='/xss/no-cross?body=hello-world'>here</a>");
        }
        ResponseEntity<String> resp = new ResponseEntity(body, HttpStatus.OK);
        return resp;
    }

    @GetMapping({"/xss/direct/5"})
    public ResponseEntity<String> noDeepCross5(@RequestParam(required = false) String body) {
        if (body == null) {
            return ResponseEntity.ok("No input, try <a href='/xss/no-cross?body=hello-world'>here</a>");
        }
        body = "Pre Handle" + body;
        body = body.replaceAll("Hello", "---Hello---");
        body += "\n\nSigned by DeepCrossController";
        ResponseEntity<String> resp = new ResponseEntity(body, HttpStatus.OK);
        return resp;
    }

    @GetMapping({"/xss/direct/6"})
    public ResponseEntity<String> noDeepCross6(@RequestParam(required = false) String body) {
        if (body == null) {
            return ResponseEntity.ok("No input, try <a href='/xss/no-cross?body=hello-world'>here</a>");
        }
        body = body.replaceAll("Hello", "---Hello---");
        body += "\n\nSigned by DeepCrossController";
        body = DummyUtil.filterXSS(body);
        ResponseEntity<String> resp = new ResponseEntity(body, HttpStatus.OK);
        return resp;
    }
}


`
	vf.AddFile("xss.java", code)
	rule := `
*?{opcode:return} as $sink;
$sink #-> ?{opcode: param} as $result;
// $sink #{
    // until: "*?{opcode: param} as $source",
// }->;
	`

	progName := uuid.NewString()
	compileStart := time.Now()
	prog, err := ssaapi.ParseProject(
		ssaapi.WithFileSystem(vf),
		ssaapi.WithProgramName(progName),
		ssaapi.WithMemory(),
	)
	compile := time.Since(compileStart)
	require.NoError(t, err)
	prog.Show()

	queryStart := time.Now()
	result, err := prog.SyntaxFlowWithError(rule, ssaapi.QueryWithProcessCallback(func(f float64, s string) {
		log.Infof("Progress: %.2f%%, Status: %s", f*100, s)
	}))
	query := time.Since(queryStart)
	require.NoError(t, err)
	require.NotNil(t, result)
	result.GetValues("result").Show()
	log.Infof("Time: \n\tCompile time: %s, \n\tQuery time: %s, \n\tTotal time: %s", compile, query, compile+query)
	diagnostics.LogCompileSummary()
}

func TestA(t *testing.T) {
	t.Skip()
	go func() {
		err := http.ListenAndServe(":18080", nil)
		if err != nil {
			return
		}
	}()
	// diagnostics.StartHeapMonitor(time.Second, diagnostics.WithFileName("heap.prof"))

	path := "/Users/wlz/Developer/Target/yakssaExample/wanwu"
	fs := filesys.NewRelLocalFs(path)

	ctx := context.Background()

	config, err := ssaapi.DefaultConfig(
		ssaapi.WithContext(ctx),
		ssaapi.WithFileSystem(fs),
		ssaapi.WithLanguage(ssaconfig.GO),
		ssaapi.WithProcess(func(msg string, process float64) {
			log.Infof("Process: %s, %.2f%%", msg, process*100)
		}),
		ssaapi.WithConcurrency(runtime.NumCPU()),
	)
	require.NoError(t, err)

	fileList := make([]string, 0)
	fileMap := make(map[string]struct{})
	diagnostics.Track(true, "collect file", func() {
		filesys.Recursive(".",
			filesys.WithFileSystem(fs),
			filesys.WithFileStat(func(s string, fi os.FileInfo) error {
				if fi.IsDir() {
					return nil
				}
				// log.Infof("file: %s", s)
				if fs.Ext(s) == ".go" {
					fileList = append(fileList, s)
					fileMap[s] = struct{}{}
				}
				return nil
			}),
		)
	})

	var ch <-chan *ssareducer.FileContent

	diagnostics.Track(true, "getFileHandler", func() {
		ch = config.GetFileHandler(
			fs,
			fileList,
			fileMap,
		)
	})

	for fc := range ch {
		if fc.Err != nil {
			log.Errorf("file %s error: %v", fc.Path, fc.Err)
			// } else {
			// 	log.Infof("file %s ", fc.Path)
		}
	}
	diagnostics.LogCompileSummary()
}
