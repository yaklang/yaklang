package csharp2ssa

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

func TestGlobalUsingRegistryResetsWithProjectPartition(t *testing.T) {
	builder := CreateBuilder().(*SSABuilder)

	first := filesys.NewVirtualFs()
	first.AddFile("Globals.cs", `
global using Demo.Domain;
global using Alias = Demo.Domain.Payload;
global using static Demo.Utility.Helpers;
`)
	builder.PartitionCompileUnits(first, []string{"Globals.cs"})
	registered := builder.globalUsings.snapshot()
	require.Equal(t, []string{"Demo.Domain"}, registered.namespaces)
	require.Equal(t, []string{"Demo.Utility.Helpers"}, registered.statics)
	require.Equal(t, "Demo.Domain.Payload", registered.aliases["Alias"])
	require.NotNil(t, registered.aliasTypes["Alias"])

	second := filesys.NewVirtualFs()
	second.AddFile("Local.cs", `using Demo.Local; class Local { }`)
	builder.PartitionCompileUnits(second, []string{"Local.cs"})
	reset := builder.globalUsings.snapshot()
	require.Empty(t, reset.namespaces)
	require.Empty(t, reset.statics)
	require.Empty(t, reset.aliases)
	require.Empty(t, reset.aliasTypes)
}

func TestScanGlobalUsingDirectivesUsesCSharpTokens(t *testing.T) {
	source := `
/*
global using Fake.FromComment;
*/
var text = @"first line
global using Fake.FromString;";

global
using Demo.Domain;
global /* legal trivia */ using static Demo.Utility.Helpers;
global using Alias =
    Demo.Domain.Payload;
`

	builder := CreateBuilder().(*SSABuilder)
	builder.prepareGlobalUsings(scanCSharpGlobalUsingDirectives(source))
	registered := builder.globalUsings.snapshot()

	require.Equal(t, []string{"Demo.Domain"}, registered.namespaces)
	require.Equal(t, []string{"Demo.Utility.Helpers"}, registered.statics)
	require.Equal(t, "Demo.Domain.Payload", registered.aliases["Alias"])
	require.NotContains(t, registered.namespaces, "Fake.FromComment")
	require.NotContains(t, registered.namespaces, "Fake.FromString")
}

func TestScanCSharpFileUsesTokensForNamespaceAndUsings(t *testing.T) {
	scan := scanCSharpFile(`
/*
namespace Fake.FromComment { using Fake.CommentImport; }
global using Fake.CommentGlobal;
*/
global
using Demo.Shared;
using
    Demo.Root;
using RootAlias = global::Demo.Aliases.Payload;
using static
    Demo.Utility.Helpers;
namespace
    Real.Project
{
    using /* legal trivia */
        Demo.Inner;

    namespace Nested {
        class NestedType { }
    }

    class Consumer {
        const string Noise = @"namespace Fake.FromString { using Fake.StringImport; }";
        void Run() {
            using var handle = Open();
        }
    }
}
`)

	require.Equal(t, "Real.Project", scan.namespaceName)
	require.Equal(t, []string{"Real.Project", "Real.Project.Nested"}, scan.namespaceNames)
	require.Equal(t, []csharpScannedUsing{
		{target: "Demo.Shared", global: true},
		{target: "Demo.Root"},
		{target: "global::Demo.Aliases.Payload"},
		{target: "Demo.Utility.Helpers"},
		{target: "Demo.Inner"},
	}, scan.usings)
	require.Len(t, scan.globalDirectives, 1)
	require.Contains(t, scan.globalDirectives[0], "Demo . Shared")

	fileScoped := scanCSharpFile(`
namespace Real.FileScoped;
using
    Demo.AfterNamespace;
class Consumer { }
`)
	require.Equal(t, "Real.FileScoped", fileScoped.namespaceName)
	require.Equal(t, []string{"Real.FileScoped"}, fileScoped.namespaceNames)
	require.Equal(t, []csharpScannedUsing{{target: "Demo.AfterNamespace"}}, fileScoped.usings)
}

func TestCSharpCompileUnitDependenciesResolveNestedAndSiblingNamespaces(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("target/Combined.cs", `
namespace Demo.First {
    namespace Nested { public class NestedTarget { } }
}
namespace Demo.Second { public class SiblingTarget { } }
`)
	vf.AddFile("consumer/Consumer.cs", `
using Demo.First.Nested;
using Demo.Second;
namespace Demo.Consumer { public class Consumer { } }
`)

	builder := CreateBuilder().(*SSABuilder)
	units := builder.PartitionCompileUnits(vf, []string{"consumer/Consumer.cs", "target/Combined.cs"})
	edges := builder.CompileUnitDependencies(vf, units)

	// Partition keeps the first namespace as the physical unit key, while all
	// other namespaces declared by that file resolve to the same dependency.
	require.Contains(t, edges, ssa.UnitRef{
		From: "csharp:Demo.Consumer", To: "csharp:Demo.First", Kind: "using", Raw: "Demo.First.Nested",
	})
	require.Contains(t, edges, ssa.UnitRef{
		From: "csharp:Demo.Consumer", To: "csharp:Demo.First", Kind: "using", Raw: "Demo.Second",
	})
	for _, unit := range units {
		require.NotEqual(t, "csharp:Demo.Second", unit.Key)
		require.NotEqual(t, "csharp:Demo.First.Nested", unit.Key)
	}
}

func TestCSharpCompileUnitDependenciesPointFromConsumerToImportedTarget(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("shared/Shared.cs", `namespace Demo.Shared { public class Shared { } }`)
	vf.AddFile("domain/Domain.cs", `namespace Demo.Domain { public class Domain { } }`)
	vf.AddFile("globals/Globals.cs", `
global using Demo.Shared;
namespace Demo.Globals { public class Marker { } }
`)
	vf.AddFile("consumer/Consumer.cs", `
/* namespace Fake.Comment { using Fake.Import; } */
using
    Demo.Domain;
namespace
    Demo.App
{
    public class Consumer {
        const string Noise = @"namespace Fake.String; using Fake.StringImport;";
    }
}
`)

	builder := CreateBuilder().(*SSABuilder)
	units := builder.PartitionCompileUnits(vf, []string{
		"consumer/Consumer.cs",
		"domain/Domain.cs",
		"globals/Globals.cs",
		"shared/Shared.cs",
	})
	edges := builder.CompileUnitDependencies(vf, units)

	require.Contains(t, edges, ssa.UnitRef{
		From: "csharp:Demo.App", To: "csharp:Demo.Domain", Kind: "using", Raw: "Demo.Domain",
	})
	require.Contains(t, edges, ssa.UnitRef{
		From: "csharp:Demo.App", To: "csharp:Demo.Shared", Kind: "global-using-target", Raw: "Demo.Shared",
	})
	for _, edge := range edges {
		require.NotEqual(t, "global-using", edge.Kind, "global declaration units must not induce artificial back edges")
		require.False(t, edge.From == "csharp:Demo.App" && edge.To == "csharp:Demo.Globals",
			"consumer must depend on the real global import target, not its declaration file")
	}

	unitKeys := make([]string, 0, len(units))
	for _, unit := range units {
		unitKeys = append(unitKeys, unit.Key)
	}
	require.Contains(t, unitKeys, "csharp:Demo.App")
	require.NotContains(t, unitKeys, "csharp:Fake.Comment")
	require.NotContains(t, unitKeys, "csharp:Fake.String")
}
