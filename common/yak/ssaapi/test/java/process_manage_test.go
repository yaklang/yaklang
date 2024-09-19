package java

import (
	"context"
	_ "embed"
	"fmt"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestJava_ProcessManage(t *testing.T) {
	var originAstCost time.Duration
	var cancelAstCost time.Duration

	fs := filesys.NewRelLocalFs(`./badcase/`)
	ssatest.CheckProfileWithFS(fs, t, func(p ssatest.ParseStage, prog ssaapi.Programs, start time.Time) error {
		if p == ssatest.OnlyMemory {
			originAstCost = time.Since(start)
		}
		return nil
	}, ssaapi.WithLanguage(ssaapi.JAVA))

	ctx, cancel := context.WithCancel(context.Background())
	timer := time.NewTimer(originAstCost / 10)
	go func() {
		select {
		case <-timer.C:
			cancel()
		}
	}()
	ssatest.CheckProfileWithFS(fs, t, func(p ssatest.ParseStage, prog ssaapi.Programs, start time.Time) error {
		if p == ssatest.OnlyMemory {
			cancelAstCost = time.Since(start)
		}
		return nil
	}, ssaapi.WithLanguage(ssaapi.JAVA), ssaapi.WithContext(ctx))
	require.Greater(t, originAstCost, cancelAstCost*2)
	log.Info("origin ast cost: ", originAstCost)
	log.Info("Proactively stop ast cost: ", cancelAstCost)
}

func TestJava_ProcessManage_Mutli_Files(t *testing.T) {
	toCreate := []string{
		"example/src/main/java/com/example/testcontextB/a.java",
		"example/src/main/java/com/example/testcontextB/b.java",
		"example/src/main/java/com/example/testcontextA/a.java",
		"example/src/main/java/com/example/testcontextA/b.java",
		"example/src/main/java/com/example/testcontextA/c.java",
		"example/src/main/java/com/example/testcontextA/d.java",
		"example/src/main/java/com/example/testcontextA/e.java",
		"example/src/main/java/com/example/testcontextC/a.java",
		"example/src/main/java/com/example/testcontextC/b.java",
		"example/src/main/java/com/example/testcontextC/c.java",
	}

	t.Run("test mutli files stop process by context", func(t *testing.T) {
		vf := filesys.NewVirtualFs()
		for _, path := range toCreate {
			dirPath := filepath.Dir(path)
			lastDirName := filepath.Base(dirPath)
			vf.AddFile(path, fmt.Sprintf("package %s;", lastDirName))
		}

		var maxProcess float64
		programID := uuid.NewString()
		ctx, cancel := context.WithCancel(context.Background())
		_, err := ssaapi.ParseProject(vf,
			ssaapi.WithLanguage(ssaapi.JAVA),
			ssaapi.WithProgramName(programID),
			ssaapi.WithProcess(func(msg string, process float64) {
				if process >= 0.5 {
					cancel()
				}
				if process > maxProcess {
					maxProcess = process
				}
				log.Infof("message %v, process: %f", msg, process)
			}),
			ssaapi.WithContext(ctx),
		)
		assert.NoErrorf(t, err, "parse project error: %v", err)
		require.LessOrEqual(t, maxProcess, 0.5)
		file := make([]string, 0)
		dbfs := ssadb.NewIrSourceFs()
		filesys.Recursive(
			fmt.Sprintf("/%s", programID),
			filesys.WithFileSystem(dbfs),
			filesys.WithFileStat(func(s string, fi fs.FileInfo) error {
				_, path, _ := strings.Cut(s, programID+"/")
				file = append(file, path)
				return nil
			}),
		)
		require.LessOrEqual(t, len(file), 5)
	})

}
