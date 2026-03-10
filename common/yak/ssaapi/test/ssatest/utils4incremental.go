package ssatest

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssa_compile"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type IncrementalCheckStage int

const (
	IncrementalCheckStageCompile IncrementalCheckStage = iota
	IncrementalCheckStageDB
)

type IncrementalStep struct {
	Files   map[string]string
	Options []ssaconfig.Option
	Check   func(*ssaapi.ProgramOverLay, IncrementalCheckStage)
}

var initIncrementalDB sync.Once

func CheckIncrementalProgram(t *testing.T, steps ...IncrementalStep) {
	t.Helper()
	CheckIncrementalProgramWithOptions(t, []ssaconfig.Option{
		ssaapi.WithLanguage(ssaconfig.JAVA),
		ssaapi.WithEnableIncrementalCompile(true),
		ssaapi.WithContext(context.Background()),
	}, steps...)
}

func CheckIncrementalProgramWithOptions(t *testing.T, options []ssaconfig.Option, steps ...IncrementalStep) {
	t.Helper()
	require.NotEmpty(t, steps)

	initIncrementalDB.Do(func() {
		yakit.InitialDatabase()
	})

	tempDir, err := os.MkdirTemp("", "ssatest-incremental-*")
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = os.RemoveAll(tempDir)
	})

	var projectConfig *ssaconfig.Config
	var projectID uint64
	var lastProgramName string
	for stepIndex, step := range steps {
		applyStepPatchToDir(t, tempDir, step.Files)

		compileOpts := append([]ssaconfig.Option{ssaapi.WithEnableIncrementalCompile(true)}, options...)
		compileOpts = append(compileOpts, step.Options...)
		compileOpts = append(compileOpts, ssaapi.WithReCompile(true))

		cfg, err := ssaconfig.New(ssaconfig.ModeProjectCompile, compileOpts...)
		require.NoError(t, err)

		ctx := cfg.GetContext()
		if ctx == nil {
			ctx = context.Background()
		}

		language := cfg.GetLanguage()
		if language == "" {
			language = ssaconfig.JAVA
		}

		if projectConfig == nil {
			info, resolvedConfig, err := ssa_compile.ProjectAutoDetective(ctx, &ssa_compile.SSADetectConfig{
				Target:   tempDir,
				Language: string(language),
				Options:  compileOpts,
			})
			require.NoError(t, err)
			require.NotNil(t, info)
			require.NotNil(t, resolvedConfig)
			projectConfig = resolvedConfig

			if !info.ProjectExists {
				configJSON, err := projectConfig.ToJSONString()
				require.NoError(t, err)

				profileDB := consts.GetGormProfileDatabase()
				project, err := yakit.CreateSSAProject(profileDB, &ypb.CreateSSAProjectRequest{
					JSONStringConfig: configJSON,
				})
				require.NoError(t, err)
				require.NotNil(t, project)
				require.NotZero(t, project.ID)

				projectID = uint64(project.ID)
				require.NoError(t, projectConfig.Update(ssaconfig.WithProjectID(projectID)))

				projectIDForCleanup := int64(project.ID)
				t.Cleanup(func() {
					_, _ = yakit.DeleteSSAProject(profileDB, &ypb.DeleteSSAProjectRequest{
						DeleteMode: string(yakit.SSAProjectDeleteAll),
						Filter: &ypb.SSAProjectFilter{
							IDs: []int64{projectIDForCleanup},
						},
					})
				})
			} else {
				projectID = projectConfig.GetProjectID()
			}
			require.NotZero(t, projectID)
		}

		baseProgramName := strings.TrimSpace(projectConfig.GetProjectName())
		if baseProgramName == "" {
			baseProgramName = filepath.Base(tempDir)
		}
		programName := fmt.Sprintf("%s_step_%d", baseProgramName, stepIndex)

		res, err := ssa_compile.ParseProjectWithName(ctx, projectConfig, programName, compileOpts...)
		require.NoError(t, err)
		require.NotNil(t, res)
		require.NotNil(t, res.Program)
		program := res.Program
		lastProgramName = program.GetProgramName()
		require.NotEmpty(t, lastProgramName)

		irProgram, err := ssadb.GetApplicationProgram(lastProgramName)
		require.NoError(t, err)
		require.NotNil(t, irProgram)
		require.Equal(t, projectID, irProgram.ProjectID)

		enableIncrementalCompile := cfg.GetEnableIncrementalCompile()
		explicitBaseProgramName := cfg.GetBaseProgramName()
		if enableIncrementalCompile {
			if stepIndex == 0 && explicitBaseProgramName == "" {
				require.Empty(t, irProgram.BaseProgramName)
			}
			if stepIndex > 0 && explicitBaseProgramName == "" {
				require.NotEmpty(t, irProgram.BaseProgramName)
			}
		}

		if step.Check == nil {
			continue
		}

		step.Check(getOverlayForCheck(t, program), IncrementalCheckStageCompile)
		step.Check(getOverlayForCheck(t, reloadProgramFromDatabase(t, lastProgramName)), IncrementalCheckStageDB)
	}
}

func applyStepPatchToDir(t *testing.T, root string, patch map[string]string) {
	t.Helper()

	for filePath, content := range patch {
		normalizedPath := normalizeIncrementalFilePath(filePath)
		if normalizedPath == "" {
			continue
		}

		fullPath := filepath.Join(root, filepath.FromSlash(normalizedPath))
		if content == "" {
			err := os.Remove(fullPath)
			if err != nil && !os.IsNotExist(err) {
				require.NoError(t, err)
			}
			continue
		}

		require.NoError(t, os.MkdirAll(filepath.Dir(fullPath), 0o755))
		require.NoError(t, os.WriteFile(fullPath, []byte(content), 0o644))
	}
}

func getOverlayForCheck(t *testing.T, program *ssaapi.Program) *ssaapi.ProgramOverLay {
	t.Helper()
	require.NotNil(t, program)

	if overlay := program.GetOverlay(); overlay != nil {
		return overlay
	}
	return buildSingleProgramOverlay(t, program)
}

func reloadProgramFromDatabase(t *testing.T, programName string) *ssaapi.Program {
	t.Helper()
	require.NotEmpty(t, programName)

	ssaapi.ProgramCache.Remove(programName)
	program, err := ssaapi.FromDatabase(programName)
	require.NoError(t, err)
	require.NotNil(t, program)
	return program
}

func buildSingleProgramOverlay(t *testing.T, program *ssaapi.Program) *ssaapi.ProgramOverLay {
	t.Helper()
	require.NotNil(t, program)
	require.NotNil(t, program.Program)

	baseClone := *program
	diffClone := *program
	diffProgram := *program.Program
	diffProgram.FileHashMap = map[string]int{}
	program.ForEachAllFile(func(filePath string, _ *memedit.MemEditor) bool {
		normalizedPath := normalizeOverlayProgramFilePath(filePath, program.GetProgramName())
		if normalizedPath != "" {
			diffProgram.FileHashMap[normalizedPath] = 1
		}
		return true
	})
	diffClone.Program = &diffProgram

	overlay := ssaapi.NewProgramOverLay(&baseClone, &diffClone)
	require.NotNil(t, overlay)
	return overlay
}

func normalizeIncrementalFilePath(filePath string) string {
	cleanPath := path.Clean("/" + filePath)
	if cleanPath == "/" || cleanPath == "." {
		return ""
	}
	return cleanPath[1:]
}

func normalizeOverlayProgramFilePath(filePath, programName string) string {
	normalizedPath := strings.TrimPrefix(filePath, "/")
	if normalizedPath == "" {
		return ""
	}
	if programName == "" {
		return normalizedPath
	}
	prefix := programName + "/"
	if strings.HasPrefix(normalizedPath, prefix) {
		return normalizedPath[len(prefix):]
	}
	if normalizedPath == programName {
		return ""
	}
	return normalizedPath
}
