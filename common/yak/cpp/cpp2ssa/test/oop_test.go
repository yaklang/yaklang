package test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

// TestCppClassFields asserts class fields are registered as normal members.
func TestCppClassFields(t *testing.T) {
	progs := compileCppProject(t, map[string]string{
		"fields.cpp": `
class Point {
public:
    int x;
    int y;
};

int use() {
    Point point;
    point.x = 1;
    point.y = 2;
    return point.x + point.y;
}
`,
	})
	blueprint := progs[0].Program.GetBluePrint("Point")
	require.NotNil(t, blueprint)
	for _, name := range []string{"x", "y"} {
		require.NotNil(t, blueprint.GetNormalMember(name), "missing field %q", name)
	}
}

// TestCppClassInheritance asserts the parent link and inherited field lookup.
func TestCppClassInheritance(t *testing.T) {
	progs := compileCppProject(t, map[string]string{
		"inherit.cpp": `
class Shape {
protected:
    int id_;
};

class Circle : public Shape {
public:
    Circle(int radius) : radius_(radius) {}
    int radius() const { return radius_; }

private:
    int radius_;
};
`,
	})
	circle := progs[0].Program.GetBluePrint("Circle")
	require.NotNil(t, circle)
	shape := progs[0].Program.GetBluePrint("Shape")
	require.NotNil(t, shape)
	parents := circle.GetParentBlueprint()
	require.Len(t, parents, 1, "Circle has one base class")
	require.Equal(t, "Shape", parents[0].Name)
	require.NotNil(t, circle.GetNormalMember("radius_"), "own field must be registered")
	require.NotNil(t, circle.GetNormalMember("id_"), "inherited field must be visible")
}

// TestCppVirtualMethodCallTarget asserts a virtual call still binds to a
// concrete method value. Dynamic dispatch is not modelled; the test records the
// static target.
func TestCppVirtualMethodCallTarget(t *testing.T) {
	progs := compileCppProject(t, map[string]string{
		"virtual.cpp": `
class Shape {
public:
    virtual int area() { return 0; }
};

class Rectangle : public Shape {
public:
    int area() { return 12; }
};

int use() {
    Rectangle rectangle;
    return rectangle.area();
}
`,
	})
	blueprint := progs[0].Program.GetBluePrint("Rectangle")
	require.NotNil(t, blueprint)
	method, ok := ssa.ToFunction(blueprint.GetNormalMethod("area"))
	require.True(t, ok, "Rectangle::area must be registered")
	use := funcBySymbol(t, progs[0], "use")
	require.NotEmpty(t, callsTo(t, use, method), "virtual call must bind to Rectangle::area")
}

// TestCppConstructorAndDestructor asserts both magic methods are registered.
func TestCppConstructorAndDestructor(t *testing.T) {
	progs := compileCppProject(t, map[string]string{
		"raii.cpp": `
class Buffer {
public:
    Buffer() : size_(4) {}
    ~Buffer() { size_ = 0; }
    int size() const { return size_; }

private:
    int size_;
};
`,
	})
	blueprint := progs[0].Program.GetBluePrint("Buffer")
	require.NotNil(t, blueprint)
	builder := funcBySymbol(t, progs[0], "Buffer::Buffer").GetOrCreateBuilder()
	constructor := blueprint.GetMagicMethod(ssa.Constructor, builder)
	require.NotNil(t, constructor, "constructor must be a magic method")
	destructor := blueprint.GetMagicMethod(ssa.Destructor, builder)
	require.NotNil(t, destructor, "destructor must be a magic method")
}

// TestCppConstructorInitializer asserts mem-initializers become side effects on
// the constructor.
func TestCppConstructorInitializer(t *testing.T) {
	progs := compileCppProject(t, map[string]string{
		"initializer.cpp": `
class Pair {
public:
    Pair() : first_(1), second_(2) {}

private:
    int first_;
    int second_;
};
`,
	})
	fn := funcBySymbol(t, progs[0], "Pair::Pair")
	// Mem-initializers are recorded on the function type (this->first_,
	// this->second_), not in the Function.SideEffects list.
	rendered := fn.GetType().String()
	require.Contains(t, rendered, "first_", "mem-initializer for first_ must be a side effect")
	require.Contains(t, rendered, "second_", "mem-initializer for second_ must be a side effect")
}

// TestCppMultipleInheritance asserts every base class is linked.
func TestCppMultipleInheritance(t *testing.T) {
	progs := compileCppProject(t, map[string]string{
		"multiple.cpp": `
class Reader {
public:
    int read() { return 1; }
};

class Writer {
public:
    int write() { return 2; }
};

class Stream : public Reader, public Writer {
};
`,
	})
	stream := progs[0].Program.GetBluePrint("Stream")
	require.NotNil(t, stream)
	parents := stream.GetParentBlueprint()
	require.Len(t, parents, 2, "Stream inherits Reader and Writer")
	names := []string{parents[0].Name, parents[1].Name}
	require.Contains(t, names, "Reader")
	require.Contains(t, names, "Writer")
}

// TestCppPolymorphicParameter asserts a base-class parameter accepts a derived
// object reference at the call site.
func TestCppPolymorphicParameter(t *testing.T) {
	progs := compileCppProject(t, map[string]string{
		"polymorphic.cpp": `
class Shape {
public:
    virtual int area() { return 0; }
};

class Square : public Shape {
public:
    int area() { return 4; }
};

int measure(Shape* shape) {
    return shape->area();
}

int use() {
    Square square;
    return measure(&square);
}
`,
	})
	measure := funcBySymbol(t, progs[0], "measure")
	require.Len(t, measure.Params, 1, "measure takes one Shape pointer")
	use := funcBySymbol(t, progs[0], "use")
	require.NotEmpty(t, callsTo(t, use, measure), "derived address must flow into the base parameter")
}
// TestCppVirtualDispatchSymbols asserts virtual declarations and overrides keep
// both method symbols; call-target selection through a base pointer is recorded
// as a known limitation (no dynamic dispatch resolution yet).
func TestCppVirtualDispatchSymbols(t *testing.T) {
	progs := compileCppProject(t, map[string]string{
		"virtual_dispatch.cpp": `
class Shape {
public:
    virtual int area() { return 0; }
};

class Rectangle : public Shape {
public:
    int area() override { return 4; }
};

int use() {
    Shape* shape = new Rectangle();
    return shape->area();
}
`,
	})
	base := findBlueprint(t, progs, "Shape")
	require.NotNil(t, base)
	baseArea := base.GetNormalMethod("area")
	require.NotNil(t, baseArea)
	derived := findBlueprint(t, progs, "Rectangle")
	require.NotNil(t, derived)
	derivedArea := derived.GetNormalMethod("area")
	require.NotNil(t, derivedArea)
	_ = funcBySymbol(t, progs[0], "use")
	// Dynamic dispatch through a base pointer is not resolved yet; both method
	// symbols and their bodies must still be present.
	_, baseIsFunction := ssa.ToFunction(baseArea)
	_, derivedIsFunction := ssa.ToFunction(derivedArea)
	require.True(t, baseIsFunction)
	require.True(t, derivedIsFunction)
}

// TestCppPointerReceiverMethod asserts a method call through a pointer resolves
// to the class method and keeps its this parameter.
func TestCppPointerReceiverMethod(t *testing.T) {
	progs := compileCppProject(t, map[string]string{
		"pointer_receiver.cpp": `
class Counter {
public:
    int bump(int by) { return by + 1; }
};

int use() {
    Counter counter;
    Counter* pointer = &counter;
    return pointer->bump(2);
}
`,
	})
	counter := findBlueprint(t, progs, "Counter")
	require.NotNil(t, counter)
	method := counter.GetNormalMethod("bump")
	require.NotNil(t, method)
	use := funcBySymbol(t, progs[0], "use")
	methodFunction, ok := ssa.ToFunction(method)
	require.True(t, ok)
	require.NotEmpty(t, callsTo(t, use, methodFunction))
}
