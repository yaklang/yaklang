package tests

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

func TestCSharp_MultiFile_NamespaceUsingAndAlias(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("Domain/User.cs", `
namespace Demo.Domain {
    public class User {
        public string Name;
        public User(string name) { Name = name; }
    }
}`)
	vf.AddFile("Services/UserService.cs", `
using Demo.Domain;
namespace Demo.Services {
    public class UserService {
        public User Build(string name) { return new User(name); }
    }
}`)
	vf.AddFile("App/Program.cs", `
using Demo.Services;
using UserAlias = Demo.Domain.User;
namespace Demo.App {
    public class Program {
        public static void Main(string[] args) {
            var service = new UserService();
            UserAlias user = service.Build(source());
            sink(user.Name);
        }
    }
}`)

	programs, err := ssaapi.ParseProjectWithFS(
		vf,
		ssaapi.WithLanguage(ssaconfig.CSHARP),
		ssaapi.WithMemory(),
	)
	require.NoError(t, err)
	require.NotEmpty(t, programs)
	prog := programs[0]
	requireCSharpCompileErrorFree(t, prog)
	require.NotEmpty(t, prog.Ref("User"))
	require.NotEmpty(t, prog.Ref("UserService"))
	require.NotEmpty(t, prog.Ref("sink"))
	result, err := prog.SyntaxFlowWithError(`source() as $source; sink(* #-> as $origin)`)
	require.NoError(t, err)
	require.NotEmpty(t, result.GetValues("source"))
	require.NotEmpty(t, result.GetValues("origin"), "cross-file constructor/member flow must reach sink")
}

func parseCSharpProjectFiles(t *testing.T, files map[string]string) *ssaapi.Program {
	t.Helper()
	vf := filesys.NewVirtualFs()
	for path, source := range files {
		vf.AddFile(path, source)
	}
	programs, err := ssaapi.ParseProjectWithFS(
		vf,
		ssaapi.WithLanguage(ssaconfig.CSHARP),
		ssaapi.WithMemory(),
	)
	require.NoError(t, err)
	require.NotEmpty(t, programs)
	requireCSharpCompileErrorFree(t, programs[0])
	return programs[0]
}

func requireCSharpParameterTypeKind(t *testing.T, prog *ssaapi.Program, name string, kind ssa.TypeKind) *ssaapi.Value {
	t.Helper()
	for _, value := range prog.Ref(name) {
		if value.IsParameter() && value.GetTypeKind() == kind {
			return value
		}
	}
	require.Failf(t, "parameter type was not emitted", "%s does not contain type kind %v: %s", name, kind, prog.Ref(name).String())
	return nil
}

func TestCSharp_MultiFile_GlobalUsingsAreProjectWideAndOrderIndependent(t *testing.T) {
	// One file per batch makes the consumer's lazy body run in a later batch
	// than the global-using declaration and exercises the streaming path.
	t.Setenv("YAK_SSA_COMPILE_UNIT_BATCH_MIN_FILES", "1")
	t.Setenv("YAK_SSA_COMPILE_UNIT_BATCH_MIN_BYTES", "0")
	t.Setenv("YAK_SSA_COMPILE_UNIT_BATCH_MAX_FILES", "1")

	for _, globalsFirst := range []bool{true, false} {
		name := "globals-sort-after-consumer"
		globalPath := "z-globals/GlobalUsings.cs"
		consumerPath := "a-consumer/Consumer.cs"
		if globalsFirst {
			name = "globals-sort-before-consumer"
			globalPath = "a-globals/GlobalUsings.cs"
			consumerPath = "z-consumer/Consumer.cs"
		}
		t.Run(name, func(t *testing.T) {
			prog := parseCSharpProjectFiles(t, map[string]string{
				"domain/Payload.cs": `
namespace Demo.Domain {
    public class Payload { }
}`,
				"utility/Helpers.cs": `
namespace Demo.Utility {
    public class Helpers {
        public static int Answer = 42;
        public static int Forward(int value) { return value; }
    }
}`,
				globalPath: `
global
using Demo.Domain;
global using PayloadAlias = Demo.Domain.Payload;
global using StringNames = System.Collections.Generic.List<string>;
global using static Demo.Utility.Helpers;
`,
				consumerPath: `
namespace Demo.App {
    public class Consumer {
        public static void Run(Payload namespacePayload, PayloadAlias aliasPayload, StringNames names) {
            sink(Forward(source()));
            println(Answer);
        }
    }
}`,
			})

			requireCSharpBlueprintParameter(t, prog, "namespacePayload", "Demo.Domain.Payload")
			requireCSharpBlueprintParameter(t, prog, "aliasPayload", "Demo.Domain.Payload")
			names := requireCSharpParameterTypeKind(t, prog, "names", ssa.SliceTypeKind)
			require.Contains(t, names.GetType().String(), "string", "global generic alias must retain its detached type arguments")

			flow, err := prog.SyntaxFlowWithError(`source() as $source; sink(* #-> as $origin); println(* as $printed)`)
			require.NoError(t, err)
			require.NotEmpty(t, flow.GetValues("source"))
			require.NotEmpty(t, flow.GetValues("origin"), "global static method must be visible in another file's lazy body")
			require.Contains(t, flow.GetValues("printed").String(), "42", "global static field must be visible project-wide")
		})
	}
}

func TestCSharp_MultiFile_OrdinaryUsingsDoNotLeak(t *testing.T) {
	prog := parseCSharpProjectFiles(t, map[string]string{
		"domain/Payload.cs": `
namespace Demo.Domain { public class Payload { } }
namespace Demo.Utility { public class Helpers { public static int Answer = 42; } }
`,
		"a-local/LocalImports.cs": `
using System.Collections.Generic;
using LocalAlias = Demo.Domain.Payload;
using static Demo.Utility.Helpers;
namespace Demo.LocalOwner {
    public class LocalOwner {
        public static void Run(List<int> localList, LocalAlias localAlias) {
            localStaticSink(Answer);
        }
    }
}`,
		"z-isolated/NoImports.cs": `
namespace Demo.NoImports {
    public class NoImports {
        public static void Run(List<int> isolatedList, LocalAlias isolatedAlias) {
            isolatedStaticSink(Answer);
        }
    }
}`,
	})

	requireCSharpParameterTypeKind(t, prog, "localList", ssa.SliceTypeKind)
	requireCSharpBlueprintParameter(t, prog, "localAlias", "Demo.Domain.Payload")

	isolatedList := requireCSharpParameterTypeKind(t, prog, "isolatedList", ssa.ClassBluePrintTypeKind)
	isolatedListType, ok := ssa.ToBluePrintType(ssaapi.GetBareType(isolatedList.GetType()))
	require.True(t, ok)
	require.Equal(t, "List", isolatedListType.Name)

	isolatedAlias := requireCSharpParameterTypeKind(t, prog, "isolatedAlias", ssa.ClassBluePrintTypeKind)
	isolatedAliasType, ok := ssa.ToBluePrintType(ssaapi.GetBareType(isolatedAlias.GetType()))
	require.True(t, ok)
	require.Equal(t, "LocalAlias", isolatedAliasType.Name, "file-local alias must not become visible in another compilation unit")

	flow, err := prog.SyntaxFlowWithError(`localStaticSink(* as $local); isolatedStaticSink(* as $isolated)`)
	require.NoError(t, err)
	require.Contains(t, flow.GetValues("local").String(), "42")
	isolated := flow.GetValues("isolated").String()
	require.Contains(t, isolated, "Answer")
	require.NotContains(t, isolated, "42", "file-local static using leaked into the other compilation unit")
}

func TestCSharp_MultiFile_MultilineNamespaceUsingOrdersSingleFileBatches(t *testing.T) {
	// Force each namespace into an independent streaming batch. The imported
	// unit must still build first even when its path sorts after the consumer.
	t.Setenv("YAK_SSA_COMPILE_UNIT_BATCH_MIN_FILES", "1")
	t.Setenv("YAK_SSA_COMPILE_UNIT_BATCH_MIN_BYTES", "0")
	t.Setenv("YAK_SSA_COMPILE_UNIT_BATCH_MAX_FILES", "1")

	for _, targetFirst := range []bool{true, false} {
		name := "target-sorts-after-consumer"
		targetPath := "z-domain/Flow.cs"
		consumerPath := "a-consumer/Consumer.cs"
		if targetFirst {
			name = "target-sorts-before-consumer"
			targetPath = "a-domain/Flow.cs"
			consumerPath = "z-consumer/Consumer.cs"
		}
		t.Run(name, func(t *testing.T) {
			prog := parseCSharpProjectFiles(t, map[string]string{
				targetPath: `
namespace Demo.Domain {
    public class Flow {
        public static string Forward(string value) { return value; }
    }
}`,
				consumerPath: `
/* namespace Fake.FromComment { using Fake.CommentImport; } */
using
    Demo.Domain;
namespace
    Demo.App
{
    public class Consumer {
        const string Noise = @"namespace Fake.FromString; using Fake.StringImport;";
        public static void Run() {
            sink(Flow.Forward(source()));
        }
    }
}`,
			})

			flow, err := prog.SyntaxFlowWithError(`source() as $source; sink(* #-> as $origin)`)
			require.NoError(t, err)
			require.NotEmpty(t, flow.GetValues("source"))
			require.NotEmpty(t, flow.GetValues("origin"), "multiline using must order the imported namespace before its consumer")
		})
	}
}
