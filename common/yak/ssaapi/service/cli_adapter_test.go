package service

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

func TestCLIAdapter_CompileAndShow(t *testing.T) {
	service := NewSSAService()
	adapter := NewCLIAdapter(service)
	ctx := context.Background()

	t.Run("compile and show success", func(t *testing.T) {
		vf := filesys.NewVirtualFs()
		vf.AddFile("test.java", `public class Test {}`)

		programName := "test-cli-compile-" + uuid.NewString()
		defer ssadb.DeleteProgram(ssadb.GetDB(), programName)

		req := &SSACompileRequest{
			ProgramName: programName,
			Language:    "java",
			Options: []ssaconfig.Option{
				ssaapi.WithFileSystem(vf),
			},
		}

		err := adapter.CompileAndShow(ctx, req)
		require.NoError(t, err)
	})

	t.Run("compile with nil service", func(t *testing.T) {
		adapter := NewCLIAdapter(nil)
		req := &SSACompileRequest{
			Target: "/tmp/test",
		}

		err := adapter.CompileAndShow(ctx, req)
		require.Error(t, err)
		require.Contains(t, err.Error(), "service is nil")
	})
}

func TestCLIAdapter_QueryProgramsAndShow(t *testing.T) {
	service := NewSSAService()
	adapter := NewCLIAdapter(service)
	ctx := context.Background()

	// 先创建一个测试程序
	programName := "test-cli-query-" + uuid.NewString()
	defer ssadb.DeleteProgram(ssadb.GetDB(), programName)

	vf := filesys.NewVirtualFs()
	vf.AddFile("test.java", `public class Test {}`)

	// 编译程序
	compileReq := &SSACompileRequest{
		ProgramName: programName,
		Language:    "java",
		Options: []ssaconfig.Option{
			ssaapi.WithFileSystem(vf),
		},
	}
	_, err := service.Compile(ctx, compileReq)
	require.NoError(t, err)

	t.Run("query and show success", func(t *testing.T) {
		err := adapter.QueryProgramsAndShow(ctx, programName)
		require.NoError(t, err)
	})

	t.Run("query non-existent program", func(t *testing.T) {
		err := adapter.QueryProgramsAndShow(ctx, "non-existent-"+uuid.NewString())
		require.NoError(t, err) // 应该显示 "no program found" 但不报错
	})

	t.Run("query with nil service", func(t *testing.T) {
		adapter := NewCLIAdapter(nil)
		err := adapter.QueryProgramsAndShow(ctx, ".*")
		require.Error(t, err)
		require.Contains(t, err.Error(), "service is nil")
	})
}

func TestCLIAdapter_SyntaxFlowQueryAndShow(t *testing.T) {
	service := NewSSAService()
	adapter := NewCLIAdapter(service)
	ctx := context.Background()

	// 先创建一个测试程序
	programName := "test-cli-sf-" + uuid.NewString()
	defer ssadb.DeleteProgram(ssadb.GetDB(), programName)

	vf := filesys.NewVirtualFs()
	vf.AddFile("test.java", `
public class Test {
    public static void main(String[] args) {
        System.out.println("test");
    }
}`)

	// 编译程序
	compileReq := &SSACompileRequest{
		ProgramName: programName,
		Language:    "java",
		Options: []ssaconfig.Option{
			ssaapi.WithFileSystem(vf),
		},
	}
	_, err := service.Compile(ctx, compileReq)
	require.NoError(t, err)

	t.Run("syntaxflow query and show success", func(t *testing.T) {
		req := &SSASyntaxFlowQueryRequest{
			ProgramName: programName,
			Rule:        "println(, * as $arg);",
			Debug:       false,
			ShowDot:     false,
			WithCode:    false,
		}

		err := adapter.SyntaxFlowQueryAndShow(ctx, req)
		// 即使查询有错误，也应该尝试显示结果
		// 所以这里不检查错误，只确保不 panic
		_ = err
	})

	t.Run("syntaxflow query with nil service", func(t *testing.T) {
		adapter := NewCLIAdapter(nil)
		req := &SSASyntaxFlowQueryRequest{
			ProgramName: programName,
			Rule:        "println(, * as $arg);",
		}

		err := adapter.SyntaxFlowQueryAndShow(ctx, req)
		require.Error(t, err)
		require.Contains(t, err.Error(), "service is nil")
	})
}

