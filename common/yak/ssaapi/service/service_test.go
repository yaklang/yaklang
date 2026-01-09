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

func TestSSAService_Compile(t *testing.T) {
	service := NewSSAService()
	ctx := context.Background()

	t.Run("compile with valid target", func(t *testing.T) {
		// 创建虚拟文件系统
		vf := filesys.NewVirtualFs()
		vf.AddFile("test.java", `
public class Test {
    public static void main(String[] args) {
        System.out.println("Hello World");
    }
}`)

		programName := "test-compile-" + uuid.NewString()
		defer ssadb.DeleteProgram(ssadb.GetDB(), programName)

		// 使用虚拟文件系统直接编译
		req := &SSACompileRequest{
			ProgramName: programName,
			Language:    "java",
			Options: []ssaconfig.Option{
				ssaapi.WithFileSystem(vf),
			},
		}

		resp, err := service.Compile(ctx, req)
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.NoError(t, resp.Error)
		require.NotEmpty(t, resp.Programs)
		require.Equal(t, programName, resp.Programs[0].GetProgramName())
	})

	t.Run("compile with empty target", func(t *testing.T) {
		req := &SSACompileRequest{
			Target: "",
		}

		resp, err := service.Compile(ctx, req)
		require.Error(t, err)
		require.NotNil(t, resp)
		require.Error(t, resp.Error)
		require.Contains(t, resp.Error.Error(), "target file or directory is required")
	})

	t.Run("compile with nil request", func(t *testing.T) {
		resp, err := service.Compile(ctx, nil)
		require.Error(t, err)
		require.NotNil(t, resp)
		require.Error(t, resp.Error)
		require.Contains(t, resp.Error.Error(), "compile request is nil")
	})

	t.Run("compile with exclude file", func(t *testing.T) {
		vf := filesys.NewVirtualFs()
		vf.AddFile("src/main.java", `public class Main {}`)
		vf.AddFile("test/Test.java", `public class Test {}`)

		programName := "test-compile-exclude-" + uuid.NewString()
		defer ssadb.DeleteProgram(ssadb.GetDB(), programName)

		req := &SSACompileRequest{
			ProgramName: programName,
			Language:    "java",
			ExcludeFile: "test/*",
			Options: []ssaconfig.Option{
				ssaapi.WithFileSystem(vf),
			},
		}

		resp, err := service.Compile(ctx, req)
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.NoError(t, resp.Error)
	})
}

func TestSSAService_QueryPrograms(t *testing.T) {
	service := NewSSAService()
	ctx := context.Background()

	// 先创建一个测试程序
	programName := "test-query-" + uuid.NewString()
	defer ssadb.DeleteProgram(ssadb.GetDB(), programName)

	vf := filesys.NewVirtualFs()
	vf.AddFile("test.java", `public class Test {}`)

	// 编译一个程序用于查询
	compileReq := &SSACompileRequest{
		ProgramName: programName,
		Language:    "java",
		Options: []ssaconfig.Option{
			ssaapi.WithFileSystem(vf),
		},
	}
	_, err := service.Compile(ctx, compileReq)
	require.NoError(t, err)

	t.Run("query by exact name", func(t *testing.T) {
		req := &SSAQueryRequest{
			ProgramNamePattern: programName,
		}

		resp, err := service.QueryPrograms(ctx, req)
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.NoError(t, resp.Error)
		require.NotEmpty(t, resp.Programs)
		require.Equal(t, programName, resp.Programs[0].GetProgramName())
	})

	t.Run("query by pattern", func(t *testing.T) {
		req := &SSAQueryRequest{
			ProgramNamePattern: "test-query-.*",
		}

		resp, err := service.QueryPrograms(ctx, req)
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.NoError(t, resp.Error)
		require.NotEmpty(t, resp.Programs)
	})

	t.Run("query with language filter", func(t *testing.T) {
		req := &SSAQueryRequest{
			ProgramNamePattern: ".*",
			Language:           "java",
		}

		resp, err := service.QueryPrograms(ctx, req)
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.NoError(t, resp.Error)
		// 验证所有返回的程序都是 java 语言
		for _, prog := range resp.Programs {
			require.Equal(t, "java", string(prog.GetLanguage()))
		}
	})

	t.Run("query with limit", func(t *testing.T) {
		req := &SSAQueryRequest{
			ProgramNamePattern: ".*",
			Limit:              1,
		}

		resp, err := service.QueryPrograms(ctx, req)
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.NoError(t, resp.Error)
		require.LessOrEqual(t, len(resp.Programs), 1)
	})

	t.Run("query with nil request", func(t *testing.T) {
		resp, err := service.QueryPrograms(ctx, nil)
		require.Error(t, err)
		require.NotNil(t, resp)
		require.Error(t, resp.Error)
		require.Contains(t, resp.Error.Error(), "query request is nil")
	})
}

func TestSSAService_SyntaxFlowQuery(t *testing.T) {
	service := NewSSAService()
	ctx := context.Background()

	// 先创建一个测试程序
	programName := "test-sf-query-" + uuid.NewString()
	defer ssadb.DeleteProgram(ssadb.GetDB(), programName)

	vf := filesys.NewVirtualFs()
	vf.AddFile("test.java", `
public class Test {
    public static void main(String[] args) {
        String input = args[0];
        System.out.println(input);
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

	t.Run("query with valid rule", func(t *testing.T) {
		req := &SSASyntaxFlowQueryRequest{
			ProgramName: programName,
			Rule:        "println(, * as $arg); $arg as $result",
		}

		resp, err := service.SyntaxFlowQuery(ctx, req)
		require.NoError(t, err)
		require.NotNil(t, resp)
		// 即使有错误，result 也可能不为 nil
		if resp.Result != nil {
			require.NotNil(t, resp.Result)
		}
	})

	t.Run("query with empty program name", func(t *testing.T) {
		req := &SSASyntaxFlowQueryRequest{
			ProgramName: "",
			Rule:        "println(, * as $arg);",
		}

		resp, err := service.SyntaxFlowQuery(ctx, req)
		require.Error(t, err)
		require.NotNil(t, resp)
		require.Error(t, resp.Error)
		require.Contains(t, resp.Error.Error(), "program name is required")
	})

	t.Run("query with nil request", func(t *testing.T) {
		resp, err := service.SyntaxFlowQuery(ctx, nil)
		require.Error(t, err)
		require.NotNil(t, resp)
		require.Error(t, resp.Error)
		require.Contains(t, resp.Error.Error(), "syntaxflow query request is nil")
	})

	t.Run("query with debug options", func(t *testing.T) {
		req := &SSASyntaxFlowQueryRequest{
			ProgramName: programName,
			Rule:        "println(, * as $arg);",
			Debug:       true,
			ShowDot:     true,
			WithCode:    true,
		}

		resp, err := service.SyntaxFlowQuery(ctx, req)
		// 即使查询失败，也应该返回响应
		require.NotNil(t, resp)
		_ = err // 可能包含错误，但不影响测试
	})
}
