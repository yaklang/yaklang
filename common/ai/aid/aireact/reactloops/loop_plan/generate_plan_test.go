package loop_plan

import (
	"bytes"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func splitTaskSections(output string) []string {
	trimmed := strings.TrimSpace(output)
	if trimmed == "" {
		return nil
	}
	raw := strings.Split(trimmed, "\n\n")
	var result []string
	for _, s := range raw {
		s = strings.TrimSpace(s)
		if s != "" {
			result = append(result, s)
		}
	}
	if len(result) == 0 && trimmed != "" {
		result = append(result, trimmed)
	}
	return result
}

func runPlanTasksStreamHandler(t *testing.T, input string) string {
	t.Helper()
	pr, pw := io.Pipe()
	go func() {
		defer pw.Close()
		data := []byte(input)
		chunkSize := 64
		for i := 0; i < len(data); i += chunkSize {
			end := i + chunkSize
			if end > len(data) {
				end = len(data)
			}
			pw.Write(data[i:end])
		}
	}()
	var buf bytes.Buffer
	planTasksStreamHandler(pr, &buf)
	return buf.String()
}

func runWithTimeout(t *testing.T, input string, timeout time.Duration) (string, bool) {
	t.Helper()
	done := make(chan string, 1)
	go func() {
		done <- runPlanTasksStreamHandler(t, input)
	}()
	select {
	case output := <-done:
		return output, true
	case <-time.After(timeout):
		return "", false
	}
}

func TestPlanTasksStreamHandler_NormalOrder(t *testing.T) {
	input := `[{"subtask_name":"design_survey","subtask_goal":"Design a user research survey","depends_on":[]}]`
	output := runPlanTasksStreamHandler(t, input)
	assert.Contains(t, output, "[design_survey]")
	assert.Contains(t, output, ": Design a user research survey")
	assert.NotContains(t, output, "(depends:")
}

func TestPlanTasksStreamHandler_WithDeps(t *testing.T) {
	input := `[{"subtask_name":"deploy_env","subtask_goal":"Deploy test environment","depends_on":["setup_tools"]}]`
	output := runPlanTasksStreamHandler(t, input)
	assert.Contains(t, output, "[deploy_env]")
	assert.Contains(t, output, ": Deploy test environment")
	assert.Contains(t, output, "(depends: setup_tools)")
}

func TestPlanTasksStreamHandler_WithIdentifier(t *testing.T) {
	input := `[{"subtask_name":"setup_env","subtask_identifier":"setup_dev_env","subtask_goal":"Setup development environment","depends_on":[]}]`
	output := runPlanTasksStreamHandler(t, input)
	assert.Contains(t, output, "[setup_env]")
	assert.Contains(t, output, "#setup_dev_env")
	assert.Contains(t, output, ": Setup development environment")
}

func TestPlanTasksStreamHandler_FullFields(t *testing.T) {
	input := `[{"subtask_name":"write_tests","subtask_identifier":"write_unit_tests","subtask_goal":"Write unit tests for core modules","depends_on":["setup_env"]}]`
	output := runPlanTasksStreamHandler(t, input)
	assert.Contains(t, output, "[write_tests]")
	assert.Contains(t, output, "#write_unit_tests")
	assert.Contains(t, output, ": Write unit tests for core modules")
	assert.Contains(t, output, "(depends: setup_env)")
}

func TestPlanTasksStreamHandler_MultipleSubtasks(t *testing.T) {
	input := `[
		{"subtask_name":"task_a","subtask_goal":"Goal for task A","depends_on":[]},
		{"subtask_name":"task_b","subtask_goal":"Goal for task B","depends_on":["task_a"]},
		{"subtask_name":"task_c","subtask_goal":"Goal for task C","depends_on":["task_a","task_b"]}
	]`
	output := runPlanTasksStreamHandler(t, input)

	sections := splitTaskSections(output)
	require.Len(t, sections, 3, "expected 3 task sections, got: %q", output)

	assert.Contains(t, sections[0], "[task_a]")
	assert.Contains(t, sections[0], "Goal for task A")
	assert.NotContains(t, sections[0], "(depends:")

	assert.Contains(t, sections[1], "[task_b]")
	assert.Contains(t, sections[1], "Goal for task B")
	assert.Contains(t, sections[1], "(depends: task_a)")

	assert.Contains(t, sections[2], "[task_c]")
	assert.Contains(t, sections[2], "Goal for task C")
	assert.Contains(t, sections[2], "(depends: task_a, task_b)")
}

func TestPlanTasksStreamHandler_MissingDependsOn(t *testing.T) {
	input := `[{"subtask_name":"standalone","subtask_goal":"A standalone task"}]`
	output, ok := runWithTimeout(t, input, 5*time.Second)
	require.True(t, ok, "handler should not block when depends_on is missing")
	assert.Contains(t, output, "[standalone]")
	assert.Contains(t, output, "A standalone task")
}

func TestPlanTasksStreamHandler_MissingSubtaskName(t *testing.T) {
	input := `[{"subtask_goal":"Goal without name","depends_on":[]}]`
	output, ok := runWithTimeout(t, input, 5*time.Second)
	require.True(t, ok, "handler should not block when subtask_name is missing")
	assert.Contains(t, output, "Goal without name")
}

func TestPlanTasksStreamHandler_EmptyArray(t *testing.T) {
	input := `[]`
	output := runPlanTasksStreamHandler(t, input)
	assert.Empty(t, output, "empty array should produce no output")
}

func TestPlanTasksStreamHandler_EmptyInput(t *testing.T) {
	output := runPlanTasksStreamHandler(t, "")
	assert.Empty(t, output, "empty input should produce no output")
}

func TestPlanTasksStreamHandler_MultipleDeps(t *testing.T) {
	input := `[{"subtask_name":"final","subtask_goal":"Final integration","depends_on":["step1","step2","step3"]}]`
	output := runPlanTasksStreamHandler(t, input)
	assert.Contains(t, output, "(depends: step1, step2, step3)")
}

func TestPlanTasksStreamHandler_UnicodeContent(t *testing.T) {
	input := `[{"subtask_name":"设计调研问卷","subtask_goal":"设计一份用户调研问卷，覆盖核心功能需求","depends_on":[]}]`
	output := runPlanTasksStreamHandler(t, input)
	assert.Contains(t, output, "设计调研问卷")
	assert.Contains(t, output, "设计一份用户调研问卷")
}

func TestPlanTasksStreamHandler_MalformedJSON(t *testing.T) {
	input := `[{"subtask_name":"broken","subtask_goal":"some goal","depends_on":`
	_, ok := runWithTimeout(t, input, 5*time.Second)
	require.True(t, ok, "handler should not block on malformed JSON")
}

func TestPlanTasksStreamHandler_ByteByByteInput(t *testing.T) {
	input := `[{"subtask_name":"slow_task","subtask_goal":"A slowly streamed goal","depends_on":["dep1"]}]`

	pr, pw := io.Pipe()
	go func() {
		defer pw.Close()
		for i := 0; i < len(input); i++ {
			pw.Write([]byte{input[i]})
			time.Sleep(time.Millisecond)
		}
	}()

	var buf bytes.Buffer
	done := make(chan struct{})
	go func() {
		defer close(done)
		planTasksStreamHandler(pr, &buf)
	}()

	select {
	case <-done:
	case <-time.After(15 * time.Second):
		t.Fatal("handler blocked on byte-by-byte input")
	}

	output := buf.String()
	assert.Contains(t, output, "[slow_task]")
	assert.Contains(t, output, "A slowly streamed goal")
	assert.Contains(t, output, "(depends: dep1)")
}

func TestPlanTasksStreamHandler_ChunkedInput(t *testing.T) {
	input := `[{"subtask_name":"chunked","subtask_goal":"This is a chunked goal that arrives in pieces","depends_on":["pre_task"]}]`

	pr, pw := io.Pipe()
	go func() {
		defer pw.Close()
		chunkSize := 10
		for i := 0; i < len(input); i += chunkSize {
			end := i + chunkSize
			if end > len(input) {
				end = len(input)
			}
			pw.Write([]byte(input[i:end]))
			time.Sleep(5 * time.Millisecond)
		}
	}()

	var buf bytes.Buffer
	done := make(chan struct{})
	go func() {
		defer close(done)
		planTasksStreamHandler(pr, &buf)
	}()

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("handler blocked on chunked input")
	}

	output := buf.String()
	assert.Contains(t, output, "[chunked]")
	assert.Contains(t, output, "This is a chunked goal that arrives in pieces")
	assert.Contains(t, output, "(depends: pre_task)")
}

func TestPlanTasksStreamHandler_StreamInterruption(t *testing.T) {
	pr, pw := io.Pipe()

	go func() {
		partial := `[{"subtask_name":"interrupted","subtask_goal":"This goal will be cut`
		pw.Write([]byte(partial))
		time.Sleep(50 * time.Millisecond)
		pw.CloseWithError(io.ErrUnexpectedEOF)
	}()

	var buf bytes.Buffer
	done := make(chan struct{})
	go func() {
		defer close(done)
		planTasksStreamHandler(pr, &buf)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("handler blocked on interrupted stream")
	}
}

func TestPlanTasksStreamHandler_GoalWithEscapedChars(t *testing.T) {
	input := `[{"subtask_name":"escape_test","subtask_goal":"Goal with \"quotes\" and \\backslashes\\","depends_on":[]}]`
	output := runPlanTasksStreamHandler(t, input)
	assert.Contains(t, output, "[escape_test]")
	assert.Contains(t, output, `"quotes"`)
	assert.Contains(t, output, `\backslashes\`)
}

func TestPlanTasksStreamHandler_EmptySubtaskGoal(t *testing.T) {
	input := `[{"subtask_name":"empty_goal","subtask_goal":"","depends_on":[]}]`
	output := runPlanTasksStreamHandler(t, input)
	assert.Contains(t, output, "[empty_goal]")
}

func TestPlanTasksStreamHandler_OnlyDependsOn(t *testing.T) {
	input := `[{"depends_on":["some_dep"]}]`
	_, ok := runWithTimeout(t, input, 5*time.Second)
	require.True(t, ok, "handler should not block when only depends_on is present")
}

func TestPlanTasksStreamHandler_InvalidDependsOnType(t *testing.T) {
	input := `[{"subtask_name":"bad_deps","subtask_goal":"Goal here","depends_on":"not_an_array"}]`
	output, ok := runWithTimeout(t, input, 5*time.Second)
	require.True(t, ok, "handler should not block on invalid depends_on type")
	assert.Contains(t, output, "[bad_deps]")
	assert.Contains(t, output, "Goal here")
}

func TestPlanTasksStreamHandler_LargeNumberOfSubtasks(t *testing.T) {
	var sb strings.Builder
	sb.WriteString("[")
	count := 50
	for i := 0; i < count; i++ {
		if i > 0 {
			sb.WriteString(",")
		}
		dep := "[]"
		if i > 0 {
			dep = `["task_prev"]`
		}
		sb.WriteString(`{"subtask_name":"task_` + string(rune('A'+i%26)) + `","subtask_goal":"Goal ` + string(rune('A'+i%26)) + `","depends_on":` + dep + `}`)
	}
	sb.WriteString("]")

	output, ok := runWithTimeout(t, sb.String(), 30*time.Second)
	require.True(t, ok, "handler blocked with large number of subtasks")
	for i := 0; i < count; i++ {
		assert.Contains(t, output, "[task_"+string(rune('A'+i%26))+"]")
	}
}

func TestPlanTasksStreamHandler_SequentialOutput(t *testing.T) {
	input := `[
		{"subtask_name":"first","subtask_goal":"First task goal","depends_on":[]},
		{"subtask_name":"second","subtask_goal":"Second task goal","depends_on":["first"]}
	]`
	output := runPlanTasksStreamHandler(t, input)

	firstIdx := strings.Index(output, "[first]")
	secondIdx := strings.Index(output, "[second]")
	require.Greater(t, firstIdx, -1, "first subtask not found in output")
	require.Greater(t, secondIdx, -1, "second subtask not found in output")
	assert.Less(t, firstIdx, secondIdx, "first subtask should appear before second in output")
}

func TestPlanTasksStreamHandler_WithIdentifierAndDeps(t *testing.T) {
	input := `[
		{"subtask_name":"setup","subtask_identifier":"setup_env","subtask_goal":"Setup environment","depends_on":[]},
		{"subtask_name":"build","subtask_identifier":"build_project","subtask_goal":"Build the project","depends_on":["setup"]}
	]`
	output := runPlanTasksStreamHandler(t, input)

	assert.Contains(t, output, "[setup]")
	assert.Contains(t, output, "#setup_env")
	assert.Contains(t, output, ": Setup environment")

	assert.Contains(t, output, "[build]")
	assert.Contains(t, output, "#build_project")
	assert.Contains(t, output, ": Build the project")
	assert.Contains(t, output, "(depends: setup)")
}

func TestPlanTasksStreamHandler_BackwardCompat_OldFormatWithoutNewFields(t *testing.T) {
	input := `[
		{"subtask_name":"配置工具","subtask_goal":"安装并配置静态代码分析工具"},
		{"subtask_name":"集成CI","subtask_goal":"修改CI配置添加检查步骤"},
		{"subtask_name":"编写文档","subtask_goal":"编写使用文档"}
	]`
	output := runPlanTasksStreamHandler(t, input)

	sections := splitTaskSections(output)
	require.Len(t, sections, 3, "expected 3 task sections for 3 subtasks without new fields, got: %q", output)

	assert.Contains(t, sections[0], "[配置工具]")
	assert.Contains(t, sections[0], "安装并配置静态代码分析工具")
	assert.NotContains(t, sections[0], "#")
	assert.NotContains(t, sections[0], "(depends:")

	assert.Contains(t, sections[1], "[集成CI]")
	assert.NotContains(t, sections[1], "#")

	assert.Contains(t, sections[2], "[编写文档]")
}

func TestPlanTasksStreamHandler_BackwardCompat_SingleTaskNoNewFields(t *testing.T) {
	input := `[{"subtask_name":"simple_task","subtask_goal":"A simple goal without depends_on or identifier"}]`
	output, ok := runWithTimeout(t, input, 5*time.Second)
	require.True(t, ok, "should complete without blocking")
	assert.Contains(t, output, "[simple_task]")
	assert.Contains(t, output, "A simple goal without depends_on or identifier")
	assert.NotContains(t, output, "#")
}

func TestPlanTasksStreamHandler_OldFormatSimulatingRealAIOutput(t *testing.T) {
	input := `[{"subtask_name":"扫描目录结构","subtask_goal":"递归遍历目录下所有文件，记录每个文件的位置和占用空间"},{"subtask_name":"计算文件大小","subtask_goal":"遍历所有文件，计算每个文件的大小"}]`
	output := runPlanTasksStreamHandler(t, input)
	assert.Contains(t, output, "[扫描目录结构]")
	assert.Contains(t, output, "[计算文件大小]")
	assert.NotContains(t, output, "#")
	assert.NotContains(t, output, "(depends:")
}

func TestPlanTasksStreamHandler_NewFormat_FullExample(t *testing.T) {
	input := `[
		{"subtask_name":"配置静态代码分析工具","subtask_identifier":"setup_static_analysis","subtask_goal":"安装并配置静态代码分析工具","depends_on":[]},
		{"subtask_name":"集成到CI/CD流程","subtask_identifier":"integrate_cicd","subtask_goal":"修改CI/CD配置文件","depends_on":["配置静态代码分析工具"]},
		{"subtask_name":"编写检查结果处理文档","subtask_identifier":"write_check_docs","subtask_goal":"编写代码质量检查工具的使用文档","depends_on":["集成到CI/CD流程"]}
	]`
	output := runPlanTasksStreamHandler(t, input)

	sections := splitTaskSections(output)
	require.Len(t, sections, 3, "expected 3 task sections, got: %q", output)

	assert.Contains(t, sections[0], "[配置静态代码分析工具]")
	assert.Contains(t, sections[0], "#setup_static_analysis")
	assert.Contains(t, sections[0], "安装并配置静态代码分析工具")
	assert.NotContains(t, sections[0], "(depends:")

	assert.Contains(t, sections[1], "[集成到CI/CD流程]")
	assert.Contains(t, sections[1], "#integrate_cicd")
	assert.Contains(t, sections[1], "(depends: 配置静态代码分析工具)")

	assert.Contains(t, sections[2], "[编写检查结果处理文档]")
	assert.Contains(t, sections[2], "#write_check_docs")
	assert.Contains(t, sections[2], "(depends: 集成到CI/CD流程)")
}

func TestPlanTasksStreamHandler_MixedFormat_SomeWithSomeWithout(t *testing.T) {
	input := `[
		{"subtask_name":"task_with_all","subtask_identifier":"full_task","subtask_goal":"has all fields","depends_on":["some_dep"]},
		{"subtask_name":"task_without_id","subtask_goal":"no identifier field","depends_on":[]},
		{"subtask_name":"task_minimal","subtask_goal":"minimal old format"}
	]`
	output := runPlanTasksStreamHandler(t, input)
	assert.Contains(t, output, "[task_with_all]")
	assert.Contains(t, output, "#full_task")
	assert.Contains(t, output, "(depends: some_dep)")

	assert.Contains(t, output, "[task_without_id]")
	assert.Contains(t, output, "no identifier field")

	assert.Contains(t, output, "[task_minimal]")
	assert.Contains(t, output, "minimal old format")
}
