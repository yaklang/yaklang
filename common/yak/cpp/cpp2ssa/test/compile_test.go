package test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

// TestCppFixturesLowerToSSA compiles every top-level fixture through the SSA
// builder. It guards lowering (not just grammar) and catches panics on the
// accepted syntax.
func TestCppFixturesLowerToSSA(t *testing.T) {
	entries, err := codeFS.ReadDir("code")
	require.NoError(t, err)
	count := 0
	for _, entry := range entries {
		if entry.IsDir() || !isCppSource(entry.Name()) {
			continue
		}
		count++
		name := entry.Name()
		t.Run(name, func(t *testing.T) {
			raw, err := codeFS.ReadFile("code/" + name)
			require.NoError(t, err)
			vf := filesys.NewVirtualFs()
			vf.AddFile(name, string(raw))
			progs, err := ssaapi.ParseProjectWithFS(
				vf,
				ssaapi.WithLanguage(ssaconfig.CPP),
				ssaapi.WithMemory(),
			)
			require.NoError(t, err)
			require.NotEmpty(t, progs)
			require.Positive(t, progs[0].Program.Funcs.Len(), "fixture lowered no function")
		})
	}
	require.Positive(t, count, "no C++ fixtures found")
}

func isCppSource(name string) bool {
	return len(name) > 4 && (name[len(name)-4:] == ".cpp" || name[len(name)-4:] == ".cxx" ||
		(len(name) > 3 && name[len(name)-3:] == ".cc"))
}

// TestCppCompileClassAndInheritance checks that a class, its base list, its
// constructors and destructors, and its methods reach the SSA.
func TestCppCompileClassAndInheritance(t *testing.T) {
	progs := compileCppProject(t, map[string]string{
		"shape.cpp": `
class Shape {
public:
    Shape() : id_(0) {}
    virtual ~Shape() {}
    virtual double area() const { return 0.0; }
    int id() const { return id_; }
protected:
    int id_;
};

class Circle : public Shape {
public:
    explicit Circle(double r) : radius_(r) {}
    double area() const { return radius_ * radius_; }
private:
    double radius_;
};

int main() {
    Circle circle(2.0);
    return circle.area() > 0.0 ? 1 : 0;
}
`,
	})
	prog := progs[0]
	for _, ref := range []string{"Shape", "Circle", "main", "circle", "radius_", "id_"} {
		require.NotEmpty(t, prog.Ref(ref), "missing SSA symbol %q", ref)
	}
	require.NotNil(t, prog.Program.GetBluePrint("Shape"))
	require.NotNil(t, prog.Program.GetBluePrint("Circle"))
}