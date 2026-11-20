package cpp2ssa

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/cfamily/preprocess"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

// newTestBuilder builds a bare SSA program with one main function, so a
// translation unit can be lowered without a full project build. The live
// blueprint state is what the frontend controls, so tests inspect it directly.
func newTestBuilder(t *testing.T) (*singleFileBuilder, *ssa.Program) {
	t.Helper()
	program := ssa.NewTmpProgram("cpp-unit-test")
	// NewTmpProgram leaves the instruction cache unset; the function builder
	// needs it to look up the entry block.
	program.Cache = ssa.NewDBCache(nil, program, ssa.ProgramCacheMemory, 0)
	function := program.NewFunctionWithParent("main", nil)
	builder := function.GetOrCreateBuilder()
	require.NotNil(t, builder)
	require.NotNil(t, builder.CurrentBlock, "test program must start with a block")
	ssaBuilder := &singleFileBuilder{
		FunctionBuilder: builder,
		typeAliases:     make(map[string]string),
		templateTypes:   make(map[string]ssa.Type),
	}
	return ssaBuilder, program
}

func lowerCpp(t *testing.T, source string) (*singleFileBuilder, *ssa.Program) {
	t.Helper()
	builder, program := newTestBuilder(t)
	ast, err := Frontend(source)
	require.NoError(t, err)
	builder.visitTranslationUnit(ast)
	return builder, program
}

func TestClassMembersRegistered(t *testing.T) {
	_, program := lowerCpp(t, `
class Shape {
public:
    Shape() : id_(0) {}
    virtual ~Shape() {}
    double area() const { return 0.0; }
    int id() const { return id_; }
protected:
    int id_;
};
`)
	shape := program.GetBluePrint("Shape")
	require.NotNil(t, shape)
	require.NotNil(t, shape.GetNormalMethod("area"), "area must be registered")
	require.NotNil(t, shape.GetNormalMethod("id"), "id must be registered")
	require.NotNil(t, shape.Constructor, "constructor must be registered as a magic method")
	require.NotNil(t, shape.Destructor, "destructor must be registered as a magic method")
	require.NotNil(t, shape.GetNormalMember("id_"), "member field id_ must be registered")
}

func TestInheritanceAndOutOfClassDefinition(t *testing.T) {
	_, program := lowerCpp(t, `
class Base {
public:
    int base_value() const { return 1; }
};

class Derived : public Base {
public:
    int derived_value() const;
};

int Derived::derived_value() const {
    return base_value() + 1;
}
`)
	derived := program.GetBluePrint("Derived")
	require.NotNil(t, derived)
	require.NotNil(t, derived.GetNormalMethod("derived_value"), "out-of-class definition must land on the class")
	parents := derived.ParentBlueprints
	require.Len(t, parents, 1)
	require.Equal(t, "Base", parents[0].Name)
}

func TestTemplateDeclarationIsParsed(t *testing.T) {
	_, program := lowerCpp(t, `
template <typename T>
T max_value(T a, T b) {
    return a > b ? a : b;
}

template <class T, int N>
class Buffer {
public:
    void set(int index, T value) { data_[index] = value; }
    T get(int index) const { return data_[index]; }
private:
    T data_[N];
};
`)
	require.NotNil(t, program.GetFunction("max_value", ""), "function template must be built")
	buffer := program.GetBluePrint("Buffer")
	require.NotNil(t, buffer, "class template must be built")
	require.NotNil(t, buffer.GetNormalMethod("set"))
	require.NotNil(t, buffer.GetNormalMethod("get"))
}

func TestStdlibBlueprintMembers(t *testing.T) {
	_, program := lowerCpp(t, `
#include <string>
#include <vector>

int use() {
    std::string name = "cpp";
    std::vector<int> numbers;
    numbers.push_back(1);
    return name.size();
}
`)
	// std::string resolves to a placeholder blueprint with catalog methods.
	stringType := program.GetBluePrint("std::string")
	require.NotNil(t, stringType, "std::string placeholder must exist")
	require.NotNil(t, stringType.GetNormalMethod("size"))
	require.NotNil(t, stringType.GetNormalMethod("substr"))
}

func TestNamespaceQualifiedFunctionName(t *testing.T) {
	_, program := lowerCpp(t, `
namespace outer {
namespace inner {
int value() { return 1; }
}
}
`)
	require.NotNil(t, program.GetFunction("outer::inner::value", ""), "namespace path must qualify the function name")
}

func TestUnknownTypesBecomePlaceholders(t *testing.T) {
	_, program := lowerCpp(t, `
ExternalThing pick() {
    ExternalThing thing;
    return thing;
}
`)
	require.NotNil(t, program.GetBluePrint("ExternalThing"), "unknown type must get a placeholder blueprint")
}

// TestAngleIncludeResolvesProjectHeader locks the C++ include policy: a header
// included as <name.hpp> is resolved inside the project before the system
// include skip applies, so vendored test headers reach macro expansion.
func TestAngleIncludeResolvesProjectHeader(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("vendor/helper.hpp", "#define PROJECT_VALUE 41\n")
	project := preprocess.BuildProject(vf, DefaultPreprocessConfig())
	source := "#include <helper.hpp>\nint main() { return PROJECT_VALUE + 1; }\n"
	expanded, err := project.PreprocessTU("vendor/main.cpp", source)
	require.NoError(t, err)
	require.Contains(t, expanded, "return 41 + 1")
}

// TestAbstractArrayTemplateArgument covers the local grammar patch for unnamed
// array types such as std::unique_ptr<char[]>.
func TestAbstractArrayTemplateArgument(t *testing.T) {
	_, program := lowerCpp(t, `
#include <memory>

int use() {
    std::unique_ptr<char[]> buffer(new char[8]);
    return 0;
}
`)
	require.NotNil(t, program.GetFunction("use", ""))
}

// TestOutOfNamespaceDefinition keeps the declarator scope in the function name.
func TestOutOfNamespaceDefinition(t *testing.T) {
	_, program := lowerCpp(t, `
namespace tools {
int helper();
}

int tools::helper() {
    return 1;
}
`)
	require.NotNil(t, program.GetFunction("tools::helper", ""))
}

// TestShiftAndIndexAssignment exercises the shift-operator chain and an indexed
// assignment target; both are easy to drop silently.
func TestShiftAndIndexAssignment(t *testing.T) {
	_, program := lowerCpp(t, `
int run() {
    int values[4];
    int shifted = 1 << 3;
    values[0] = shifted >> 1;
    return values[0];
}
`)
	require.NotNil(t, program.GetFunction("run", ""))
}
