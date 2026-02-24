package loopinfra

import (
	"context"
	"fmt"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon/mock"
)

func ExampleSingleFileModificationSuiteFactory() {
	runtime := mock.NewMockInvoker(context.Background())
	factory := NewSingleFileModificationSuiteFactory(
		runtime,
		WithLoopVarsPrefix("yak"),
		WithActionSuffix("code"),
		WithFileExtension(".yak"),
		WithAITagConfig("GEN_CODE", "yak_code", "yaklang-code", "code/yaklang"),
	)

	fmt.Printf("%s,%s,%s,%s\n",
		factory.GetActionName("write"),
		factory.GetActionName("modify"),
		factory.GetActionName("insert"),
		factory.GetActionName("delete"),
	)
	fmt.Println(len(factory.GetActions()))
	// Output:
	// write_code,modify_code,insert_code,delete_code
	// 4
}

func ExampleSingleFileModificationSuiteFactory_reportGenerating() {
	runtime := mock.NewMockInvoker(context.Background())
	factory := NewSingleFileModificationSuiteFactory(
		runtime,
		WithLoopVarsPrefix("report"),
		WithActionSuffix("section"),
		WithFileExtension(".md"),
		WithExitAfterWrite(false),
	)

	fmt.Printf("%s %v\n", factory.GetActionName("write"), factory.ShouldExitAfterWrite())
	// Output:
	// write_section false
}

