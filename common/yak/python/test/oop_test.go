package test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

// ─────────────────────────────────────────────────────────────────────────────
// TestClassInstantiation — 类实例化
// ─────────────────────────────────────────────────────────────────────────────

func TestClassInstantiation(t *testing.T) {
	t.Run("simple instantiation - constructor call exists", func(t *testing.T) {
		prog := parsePython(t, `
class MyClass:
    def __init__(self, x):
        self.x = x
obj = MyClass(10)
`)
		// ClassConstructor 调用节点存在，实参包含 Undefined-placeholder 和 10
		got := sfValues(t, prog, `MyClass(* as $args) as $call`, "call")
		require.Len(t, got, 1, "should have exactly one constructor call")
		require.Contains(t, got[0], "MyClass", "call should reference MyClass constructor")
	})

	t.Run("simple instantiation - constructor arg traced via println", func(t *testing.T) {
		// __init__ 中 println(x)，调用 MyClass(42) → x=42
		ssatest.CheckSyntaxFlowPrintWithPython(t, `
class MyClass:
    def __init__(self, x):
        println(x)
obj = MyClass(42)
`, []string{"42"})
	})

	t.Run("instantiation with multiple args - each arg at correct position", func(t *testing.T) {
		prog := parsePython(t, `
class Point:
    def __init__(self, x, y):
        self.x = x
        self.y = y
p = Point(1, 2)
`)
		// Point(self_placeholder, 1, 2) — SF 的第 2、3 个 * 捕获 x=1, y=2
		got := sfValues(t, prog, `Point(* as $s, * as $x, * as $y) as $call`, "x")
		require.Equal(t, []string{"1"}, got, "x should be 1")
		got = sfValues(t, prog, `Point(* as $s, * as $x, * as $y) as $call`, "y")
		require.Equal(t, []string{"2"}, got, "y should be 2")
	})

	t.Run("instantiation with multiple args - println traces both", func(t *testing.T) {
		ssatest.CheckSyntaxFlowPrintWithPython(t, `
class Point:
    def __init__(self, x, y):
        println(x)
        println(y)
p = Point(10, 20)
`, []string{"10", "20"})
	})

	t.Run("instantiation without args - constructor call exists", func(t *testing.T) {
		prog := parsePython(t, `
class Counter:
    def __init__(self):
        self.count = 0
c = Counter()
`)
		got := sfValues(t, prog, `Counter(* as $args) as $call`, "call")
		require.Len(t, got, 1, "should have one constructor call")
	})

	t.Run("class blueprint node registered in SSA", func(t *testing.T) {
		prog := parsePython(t, `
class MyClass:
    pass
`)
		// 类蓝图节点以类名标识
		got := sfValues(t, prog, `MyClass as $cls`, "cls")
		require.Len(t, got, 1)
		require.Equal(t, "MyClass", got[0])
	})
}

// ─────────────────────────────────────────────────────────────────────────────
// TestSelfAttributeAccess — self 属性访问
// ─────────────────────────────────────────────────────────────────────────────

func TestSelfAttributeAccess(t *testing.T) {
	t.Run("self attribute assignment - println traces value", func(t *testing.T) {
		ssatest.CheckSyntaxFlowPrintWithPython(t, `
class MyClass:
    def __init__(self, value):
        self.value = value
        println(self.value)
obj = MyClass(42)
`, []string{"42"})
	})

	t.Run("self attribute access in method - ParameterMember", func(t *testing.T) {
		prog := parsePython(t, `
class MyClass:
    def __init__(self, value):
        self.value = value

    def get_value(self):
        return self.value
`)
		got := sfValues(t, prog, `MyClass.get_value as $fn`, "fn")
		require.Len(t, got, 1)
		require.Equal(t, "Function-MyClass.get_value", got[0])
	})

	t.Run("self attribute println in method - printed parameter member", func(t *testing.T) {
		// self.value 在方法中被 println 时，追踪到的是 ParameterMember（SSA 跨函数成员传播）
		prog := parsePython(t, `
class MyClass:
    def __init__(self, value):
        self.value = value

    def get_value(self):
        println(self.value)

obj = MyClass(99)
obj.get_value()
`)
		res, err := prog.SyntaxFlowWithError(`println(* as $raw)`)
		require.NoError(t, err)
		vals := res.GetValues("raw")
		require.GreaterOrEqual(t, len(vals), 1, "println should have at least 1 arg")
		// self.value 在 SSA 中表示为 ParameterMember
		found := false
		for _, v := range vals {
			if v.GetOpcode() == "ParameterMember" {
				found = true
				break
			}
		}
		require.True(t, found, "self.value should be a ParameterMember in SSA")
	})

	t.Run("multiple self attributes - both methods registered", func(t *testing.T) {
		prog := parsePython(t, `
class Rectangle:
    def __init__(self, width, height):
        self.width = width
        self.height = height

    def area(self):
        return self.width * self.height
`)
		got := sfValues(t, prog, `Rectangle.area as $fn`, "fn")
		require.Len(t, got, 1)
		require.Equal(t, "Function-Rectangle.area", got[0])
	})

	t.Run("self attribute println in __init__", func(t *testing.T) {
		ssatest.CheckSyntaxFlowPrintWithPython(t, `
class Rectangle:
    def __init__(self, width, height):
        self.width = width
        self.height = height
        println(width)
        println(height)
r = Rectangle(5, 3)
`, []string{"5", "3"})
	})
}

// ─────────────────────────────────────────────────────────────────────────────
// TestStaticMethod — @staticmethod
// ─────────────────────────────────────────────────────────────────────────────

func TestStaticMethod(t *testing.T) {
	t.Run("static method definition - function node exists", func(t *testing.T) {
		prog := parsePython(t, `
class MathUtils:
    @staticmethod
    def add(a, b):
        return a + b
result = MathUtils.add(1, 2)
`)
		got := sfValues(t, prog, `MathUtils.add as $fn`, "fn")
		require.Len(t, got, 1)
		require.Equal(t, "Function-MathUtils.add", got[0])
	})

	t.Run("static method call args - println traces a and b", func(t *testing.T) {
		ssatest.CheckSyntaxFlowPrintWithPython(t, `
class MathUtils:
    @staticmethod
    def add(a, b):
        println(a)
        println(b)
MathUtils.add(1, 2)
`, []string{"1", "2"})
	})

	t.Run("static method call site - args at position", func(t *testing.T) {
		prog := parsePython(t, `
class MathUtils:
    @staticmethod
    def add(a, b):
        return a + b
MathUtils.add(10, 20)
`)
		// 静态方法无 self，直接匹配两个实参
		got := sfValues(t, prog, `MathUtils.add(* as $a, * as $b) as $call`, "a")
		require.Equal(t, []string{"10"}, got)
		got = sfValues(t, prog, `MathUtils.add(* as $a, * as $b) as $call`, "b")
		require.Equal(t, []string{"20"}, got)
	})

	t.Run("static method in class with init - println traces val", func(t *testing.T) {
		ssatest.CheckSyntaxFlowPrintWithPython(t, `
class MyClass:
    def __init__(self, x):
        self.x = x

    @staticmethod
    def create(val):
        println(val)
        return MyClass(val)

MyClass.create(42)
`, []string{"42"})
	})
}

// ─────────────────────────────────────────────────────────────────────────────
// TestClassMethod — @classmethod
// ─────────────────────────────────────────────────────────────────────────────

func TestClassMethod(t *testing.T) {
	t.Run("classmethod definition - function node exists", func(t *testing.T) {
		prog := parsePython(t, `
class Counter:
    @classmethod
    def increment(cls):
        return cls
Counter.increment()
`)
		got := sfValues(t, prog, `Counter.increment as $fn`, "fn")
		require.Len(t, got, 1)
		require.Equal(t, "Function-Counter.increment", got[0])
	})

	t.Run("classmethod call arg - println traces val", func(t *testing.T) {
		ssatest.CheckSyntaxFlowPrintWithPython(t, `
class Counter:
    @classmethod
    def create(cls, val):
        println(val)
Counter.create(7)
`, []string{"7"})
	})

	t.Run("classmethod with default param - println traces", func(t *testing.T) {
		ssatest.CheckSyntaxFlowPrintWithPython(t, `
class Animal:
    @classmethod
    def create(cls, name, kind="dog"):
        println(name)
        println(kind)
Animal.create("Buddy", "cat")
`, []string{`"Buddy"`, `"cat"`})
	})
}

// ─────────────────────────────────────────────────────────────────────────────
// TestPropertyDecorator — @property
// ─────────────────────────────────────────────────────────────────────────────

func TestPropertyDecorator(t *testing.T) {
	t.Run("property getter - method node registered", func(t *testing.T) {
		prog := parsePython(t, `
class Circle:
    def __init__(self, radius):
        self._radius = radius

    @property
    def radius(self):
        return self._radius
c = Circle(5)
r = c.radius
`)
		got := sfValues(t, prog, `Circle.radius as $fn`, "fn")
		require.Len(t, got, 1)
		require.Equal(t, "Function-Circle.radius", got[0])
	})

	t.Run("property getter - backing field accessible as ParameterMember", func(t *testing.T) {
		prog := parsePython(t, `
class Circle:
    def __init__(self, radius):
        self._radius = radius

    @property
    def radius(self):
        return self._radius
`)
		// _radius 有两个节点：Parameter-radius（初始化参数）和 ParameterMember（成员引用）
		got := sfValues(t, prog, `Circle._radius as $field`, "field")
		require.Len(t, got, 2, "_radius should have Parameter and ParameterMember nodes")
		// 精确验证两个节点的值
		require.Equal(t, []string{"Parameter-radius", "ParameterMember-parameter[0]._radius"}, got)
	})

	t.Run("property - __init__ arg println traced", func(t *testing.T) {
		ssatest.CheckSyntaxFlowPrintWithPython(t, `
class Circle:
    def __init__(self, radius):
        self._radius = radius
        println(radius)

    @property
    def radius(self):
        return self._radius
c = Circle(5)
`, []string{"5"})
	})
}

// ─────────────────────────────────────────────────────────────────────────────
// TestSuperCall — super() 调用
// ─────────────────────────────────────────────────────────────────────────────

func TestSuperCall(t *testing.T) {
	t.Run("super init call - both class constructors traced via println", func(t *testing.T) {
		ssatest.CheckSyntaxFlowPrintWithPython(t, `
class Animal:
    def __init__(self, name):
        println(name)

class Dog(Animal):
    def __init__(self, name, breed):
        Animal.__init__(self, name)
        println(breed)

d = Dog("Rex", "Labrador")
`, []string{`"Rex"`, `"Labrador"`})
	})

	t.Run("super init - child class constructor call exists", func(t *testing.T) {
		prog := parsePython(t, `
class Animal:
    def __init__(self, name):
        self.name = name

class Dog(Animal):
    def __init__(self, name, breed):
        super().__init__(name)
        self.breed = breed

d = Dog("Rex", "Labrador")
`)
		// Dog 的构造调用存在
		got := sfValues(t, prog, `Dog(* as $args) as $call`, "call")
		require.Len(t, got, 1, "Dog constructor should be called once")
	})

	t.Run("super init - child constructor println traces breed", func(t *testing.T) {
		ssatest.CheckSyntaxFlowPrintWithPython(t, `
class Animal:
    def __init__(self, name):
        self.name = name

class Dog(Animal):
    def __init__(self, name, breed):
        super().__init__(name)
        println(breed)

d = Dog("Rex", "Labrador")
`, []string{`"Labrador"`})
	})

	t.Run("super method call - both methods registered", func(t *testing.T) {
		prog := parsePython(t, `
class Animal:
    def speak(self):
        return "..."

class Dog(Animal):
    def speak(self):
        parent_sound = super().speak()
        return parent_sound + " woof"

d = Dog()
s = d.speak()
`)
		// 用 ?{opcode: Function} 精确过滤，排除调用点产生的 Undefined 节点
		got := sfValues(t, prog, `Animal.speak?{opcode: Function} as $fn`, "fn")
		require.Len(t, got, 1)
		require.Equal(t, "Function-Animal.speak", got[0])
		got = sfValues(t, prog, `Dog.speak?{opcode: Function} as $fn`, "fn")
		require.Len(t, got, 1)
		require.Equal(t, "Function-Dog.speak", got[0])
	})
}

// ─────────────────────────────────────────────────────────────────────────────
// TestMethodCall — 实例方法调用
// ─────────────────────────────────────────────────────────────────────────────

func TestMethodCall(t *testing.T) {
	t.Run("method call - args traced via println", func(t *testing.T) {
		ssatest.CheckSyntaxFlowPrintWithPython(t, `
class Calculator:
    def add(self, a, b):
        println(a)
        println(b)
calc = Calculator()
calc.add(3, 4)
`, []string{"3", "4"})
	})

	t.Run("method call - method node exact name", func(t *testing.T) {
		// Calculator.add 有两个节点：Function 定义节点 + Undefined 调用点节点
		// 用 ?{opcode: Function} 过滤，精确得到函数定义节点
		prog := parsePython(t, `
class Calculator:
    def add(self, a, b):
        return a + b
calc = Calculator()
result = calc.add(3, 4)
`)
		got := sfValues(t, prog, `Calculator.add?{opcode: Function} as $fn`, "fn")
		require.Len(t, got, 1)
		require.Equal(t, "Function-Calculator.add", got[0])
	})

	t.Run("method call - call site args captured", func(t *testing.T) {
		// calc.add(3, 4) 在调用点有 3 个实参：self + 3 + 4
		prog := parsePython(t, `
class Calculator:
    def add(self, a, b):
        return a + b
calc = Calculator()
result = calc.add(3, 4)
`)
		got := sfValues(t, prog, `Calculator.add(* as $s, * as $a, * as $b) as $call`, "a")
		require.Equal(t, []string{"3"}, got)
		got = sfValues(t, prog, `Calculator.add(* as $s, * as $a, * as $b) as $call`, "b")
		require.Equal(t, []string{"4"}, got)
	})

	t.Run("chained method calls - method node exact name", func(t *testing.T) {
		// Builder.add 只有一个 Function 节点（无调用点 Undefined 噪声）
		prog := parsePython(t, `
class Builder:
    def __init__(self):
        self.parts = []

    def add(self, part):
        self.parts.append(part)
        return self

b = Builder()
b.add("a").add("b")
`)
		got := sfValues(t, prog, `Builder.add as $fn`, "fn")
		require.Len(t, got, 1)
		require.Equal(t, "Function-Builder.add", got[0])
	})

	t.Run("chained method - println traces part arg as Parameter", func(t *testing.T) {
		// println(part) 的直接参数是 Parameter-part（参数节点）；
		// 单次调用 b.add("hello")，#-> 追踪到 Parameter-part
		prog := parsePython(t, `
class Builder:
    def __init__(self):
        self.parts = []

    def add(self, part):
        println(part)
        return self

b = Builder()
b.add("hello")
`)
		// println 直接参数：Parameter-part
		got := sfValues(t, prog, `println(* as $raw)`, "raw")
		require.Equal(t, []string{"Parameter-part"}, got)
	})
}

// ─────────────────────────────────────────────────────────────────────────────
// TestInheritanceAndPolymorphism — 继承与多态
// ─────────────────────────────────────────────────────────────────────────────

func TestInheritanceAndPolymorphism(t *testing.T) {
	t.Run("basic inheritance - both method nodes registered", func(t *testing.T) {
		prog := parsePython(t, `
class Shape:
    def __init__(self, color):
        self.color = color

    def area(self):
        return 0

class Rectangle(Shape):
    def __init__(self, color, width, height):
        self.color = color
        self.width = width
        self.height = height

    def area(self):
        return self.width * self.height

r = Rectangle("red", 5, 3)
a = r.area()
`)
		got := sfValues(t, prog, `Shape.area as $fn`, "fn")
		require.Len(t, got, 1)
		require.Equal(t, "Function-Shape.area", got[0])

		got = sfValues(t, prog, `Rectangle.area as $fn`, "fn")
		require.Len(t, got, 1)
		require.Equal(t, "Function-Rectangle.area", got[0])
	})

	t.Run("basic inheritance - constructor args traced via println", func(t *testing.T) {
		ssatest.CheckSyntaxFlowPrintWithPython(t, `
class Rectangle:
    def __init__(self, color, width, height):
        println(color)
        println(width)
        println(height)
r = Rectangle("red", 5, 3)
`, []string{`"red"`, "5", "3"})
	})

	t.Run("multiple inheritance - all method nodes registered", func(t *testing.T) {
		prog := parsePython(t, `
class Flyable:
    def fly(self):
        return "flying"

class Swimmable:
    def swim(self):
        return "swimming"

class Duck(Flyable, Swimmable):
    def quack(self):
        return "quack"

d = Duck()
d.fly()
d.swim()
`)
		for _, tc := range []struct {
			rule   string
			wantFn string
		}{
			{`Flyable.fly?{opcode: Function} as $fn`, "Function-Flyable.fly"},
			{`Swimmable.swim?{opcode: Function} as $fn`, "Function-Swimmable.swim"},
			{`Duck.quack?{opcode: Function} as $fn`, "Function-Duck.quack"},
		} {
			got := sfValues(t, prog, tc.rule, "fn")
			require.Len(t, got, 1, "expected exactly one Function node for %s", tc.wantFn)
			require.Equal(t, tc.wantFn, got[0])
		}
	})

	t.Run("multiple inheritance - println in quack", func(t *testing.T) {
		ssatest.CheckSyntaxFlowPrintWithPython(t, `
class Duck:
    def quack(self):
        println("quack")

d = Duck()
d.quack()
`, []string{`"quack"`})
	})
}

// ─────────────────────────────────────────────────────────────────────────────
// TestCompleteOOPProgram — 综合 OOP 场景
// ─────────────────────────────────────────────────────────────────────────────

func TestCompleteOOPProgram(t *testing.T) {
	code := `
class Vehicle:
    total_vehicles = 0

    def __init__(self, make, model, year):
        self.make = make
        self.model = model
        self.year = year
        Vehicle.total_vehicles += 1

    def get_info(self):
        return self.make + " " + self.model

    @staticmethod
    def get_total():
        return Vehicle.total_vehicles

    @classmethod
    def from_string(cls, info):
        parts = info
        return cls(parts, parts, 2024)

class Car(Vehicle):
    def __init__(self, make, model, year, doors):
        self.make = make
        self.model = model
        self.year = year
        self.doors = doors

    def get_info(self):
        return self.make + " " + self.model

car = Car("Toyota", "Camry", 2023, 4)
info = car.get_info()
total = Vehicle.get_total()
`
	t.Run("parse without panic", func(t *testing.T) {
		parsePython(t, code)
	})

	t.Run("Vehicle method nodes registered", func(t *testing.T) {
		prog := parsePython(t, code)
		for _, tc := range []struct {
			rule   string
			expect string
		}{
			{`Vehicle.get_info?{opcode: Function} as $fn`, "Function-Vehicle.get_info"},
			{`Vehicle.get_total?{opcode: Function} as $fn`, "Function-Vehicle.get_total"},
			{`Vehicle.from_string?{opcode: Function} as $fn`, "Function-Vehicle.from_string"},
		} {
			got := sfValues(t, prog, tc.rule, "fn")
			require.Len(t, got, 1, "expected function node for rule: %s", tc.rule)
			require.Equal(t, tc.expect, got[0])
		}
	})

	t.Run("Car method nodes registered", func(t *testing.T) {
		prog := parsePython(t, code)
		got := sfValues(t, prog, `Car.get_info?{opcode: Function} as $fn`, "fn")
		require.Len(t, got, 1)
		require.Equal(t, "Function-Car.get_info", got[0])
	})

	t.Run("Car constructor args at call site", func(t *testing.T) {
		prog := parsePython(t, code)
		// Car("Toyota", "Camry", 2023, 4)
		got := sfValues(t, prog, `Car(* as $s, * as $make, * as $model, * as $year, * as $doors) as $call`, "make")
		require.Equal(t, []string{`"Toyota"`}, got)
		got = sfValues(t, prog, `Car(* as $s, * as $make, * as $model, * as $year, * as $doors) as $call`, "doors")
		require.Equal(t, []string{"4"}, got)
	})
}

// ─────────────────────────────────────────────────────────────────────────────
// TestClassInstantiationSyntaxFlow — SF 专项：类蓝图查找
// ─────────────────────────────────────────────────────────────────────────────

func TestClassInstantiationSyntaxFlow(t *testing.T) {
	t.Run("find class blueprint with no methods", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, `
class MyClass:
    pass
`, `MyClass as $class`, map[string][]string{
			"class": {"MyClass"},
		}, ssaapi.WithLanguage(ssaconfig.PYTHON))
	})

	t.Run("class with __init__ - blueprint node is Make", func(t *testing.T) {
		prog := parsePython(t, `
class Point:
    def __init__(self, x, y):
        self.x = x
        self.y = y
p = Point(1, 2)
`)
		// 类蓝图 Make 节点存在（类定义时生成）
		res, err := prog.SyntaxFlowWithError(`Point as $cls`)
		require.NoError(t, err)
		vals := res.GetValues("cls")
		require.GreaterOrEqual(t, len(vals), 1, "Point blueprint should exist")
		found := false
		for _, v := range vals {
			if v.GetOpcode() == "Make" {
				found = true
				break
			}
		}
		require.True(t, found, "class blueprint should have a Make node")
	})

	t.Run("multiple classes - each blueprint independently findable", func(t *testing.T) {
		prog := parsePython(t, `
class Foo:
    pass
class Bar:
    pass
class Baz:
    pass
`)
		for _, name := range []string{"Foo", "Bar", "Baz"} {
			got := sfValues(t, prog, name+` as $cls`, "cls")
			require.Len(t, got, 1, "%s blueprint should exist", name)
			require.Equal(t, name, got[0])
		}
	})
}

// ─────────────────────────────────────────────────────────────────────────────
// TestStaticMethodSyntaxFlow — SF 专项：静态方法查找
// ─────────────────────────────────────────────────────────────────────────────

func TestStaticMethodSyntaxFlow(t *testing.T) {
	t.Run("find static method definition", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, `
class Utils:
    @staticmethod
    def helper():
        pass
`, `helper as $m`, map[string][]string{
			"m": {"Function-Utils.helper"},
		}, ssaapi.WithLanguage(ssaconfig.PYTHON))
	})

	t.Run("static method with args - call site args verified", func(t *testing.T) {
		prog := parsePython(t, `
class MathUtils:
    @staticmethod
    def clamp(value, lo=0, hi=100):
        return value
MathUtils.clamp(50)
MathUtils.clamp(150, 0, 100)
`)
		got := sfValues(t, prog, `MathUtils.clamp as $fn`, "fn")
		require.Len(t, got, 1)
		require.Equal(t, "Function-MathUtils.clamp", got[0])

		// 两个调用点：clamp(50) 和 clamp(150,0,100)
		// (*, *, *) 模式：两个调用点都匹配，第一个 * 拿 value 实参
		got = sfValues(t, prog, `MathUtils.clamp(* as $v, * as $lo, * as $hi) as $call`, "v")
		require.Equal(t, []string{"150", "50"}, got) // sorted: 两个调用点的第一个实参
		got = sfValues(t, prog, `MathUtils.clamp(* as $v, * as $lo, * as $hi) as $call`, "lo")
		require.Equal(t, []string{"0"}, got) // 只有第二个调用点提供了 lo
		got = sfValues(t, prog, `MathUtils.clamp(* as $v, * as $lo, * as $hi) as $call`, "hi")
		require.Equal(t, []string{"100"}, got)
	})
}

// ─────────────────────────────────────────────────────────────────────────────
// TestOOPDataFlow — 数据流：从实例化到方法调用到返回值
// ─────────────────────────────────────────────────────────────────────────────

func TestOOPDataFlow(t *testing.T) {
	t.Run("constructor arg flows to println in init", func(t *testing.T) {
		ssatest.CheckSyntaxFlowPrintWithPython(t, `
class Car:
    def __init__(self, make, model, year):
        self.make = make
        self.model = model
        self.year = year
        println(make)
car = Car("Toyota", "Camry", 2023)
`, []string{`"Toyota"`})
	})

	t.Run("dataflow through multiple assignments - println traces origin", func(t *testing.T) {
		ssatest.CheckSyntaxFlowPrintWithPython(t, `
class Box:
    def __init__(self, w):
        self.w = w
        println(w)
b = Box(99)
`, []string{"99"})
	})

	t.Run("static method arg println", func(t *testing.T) {
		ssatest.CheckSyntaxFlowPrintWithPython(t, `
class Utils:
    @staticmethod
    def process(data):
        println(data)
Utils.process("hello")
`, []string{`"hello"`})
	})

	t.Run("classmethod arg println", func(t *testing.T) {
		ssatest.CheckSyntaxFlowPrintWithPython(t, `
class Factory:
    @classmethod
    def create(cls, name):
        println(name)
Factory.create("widget")
`, []string{`"widget"`})
	})

	t.Run("inheritance - subclass constructor args flow", func(t *testing.T) {
		ssatest.CheckSyntaxFlowPrintWithPython(t, `
class Animal:
    def __init__(self, name):
        println(name)

class Dog(Animal):
    def __init__(self, name, breed):
        Animal.__init__(self, name)
        println(breed)

d = Dog("Rex", "Husky")
`, []string{`"Rex"`, `"Husky"`})
	})

	t.Run("find taint source in constructor via #->", func(t *testing.T) {
		prog := parsePython(t, `
class Point:
    def __init__(self, x, y):
        self.x = x
        self.y = y
p = Point(100, 200)
`)
		// Point 调用节点向上溯源应包含 100 和 200
		res, err := prog.SyntaxFlowWithError(
			`Point(* as $args) as $call; $call #-> * as $src`)
		require.NoError(t, err)
		srcs := make([]string, 0)
		for _, v := range res.GetValues("src") {
			srcs = append(srcs, v.String())
		}
		requireContains(t, srcs, []string{"100", "200"})
	})
}

// ─────────────────────────────────────────────────────────────────────────────
// TestOOPBottomUse — 向下跟踪：OOP 场景中实参/调用节点被谁使用（bottom-use / -->）
// ─────────────────────────────────────────────────────────────────────────────

func TestOOPBottomUse(t *testing.T) {
	t.Run("constructor arg single-hop to constructor call", func(t *testing.T) {
		// Point(1,2)：实参 1 单跳 (->) 到 Point 的调用节点（opcode=Call）
		prog := parsePython(t, `
class Point:
    def __init__(self, x, y):
        self.x = x
        self.y = y
p = Point(1, 2)
`)
		res, err := prog.SyntaxFlowWithError(`Point(* as $s, * as $x, * as $y) as $call; $x -> * as $use`)
		require.NoError(t, err)
		vals := res.GetValues("use")
		require.Len(t, vals, 1)
		require.Equal(t, "Call", vals[0].GetOpcode())
		require.Contains(t, vals[0].String(), "Point")
	})

	t.Run("static method call arg single-hop to call node", func(t *testing.T) {
		// Utils.process("hello")："hello" 单跳到 process 调用节点
		prog := parsePython(t, `
class Utils:
    @staticmethod
    def process(data):
        println(data)
Utils.process("hello")
`)
		res, err := prog.SyntaxFlowWithError(`Utils.process(* as $d) as $call; $d -> * as $use`)
		require.NoError(t, err)
		vals := res.GetValues("use")
		require.Len(t, vals, 1)
		require.Equal(t, "Call", vals[0].GetOpcode())
		require.Contains(t, vals[0].String(), "process")
	})

	t.Run("classmethod call arg single-hop to call node", func(t *testing.T) {
		// Factory.create("widget")："widget" 单跳到 create 调用节点
		prog := parsePython(t, `
class Factory:
    @classmethod
    def create(cls, name):
        println(name)
Factory.create("widget")
`)
		res, err := prog.SyntaxFlowWithError(`Factory.create(* as $name) as $call; $name -> * as $use`)
		require.NoError(t, err)
		vals := res.GetValues("use")
		require.Len(t, vals, 1)
		require.Equal(t, "Call", vals[0].GetOpcode())
		require.Contains(t, vals[0].String(), "create")
	})

	t.Run("method call arg single-hop to method call node", func(t *testing.T) {
		// c.add(3,4)：3 单跳到 Calc.add 的调用节点
		prog := parsePython(t, `
class Calc:
    def add(self, a, b):
        println(a)
c = Calc()
c.add(3, 4)
`)
		res, err := prog.SyntaxFlowWithError(`Calc.add(* as $s, * as $a, * as $b) as $call; $a -> * as $use`)
		require.NoError(t, err)
		vals := res.GetValues("use")
		require.Len(t, vals, 1)
		require.Equal(t, "Call", vals[0].GetOpcode())
		require.Contains(t, vals[0].String(), "add")
	})

	t.Run("class blueprint used by multiple instantiations", func(t *testing.T) {
		// Point 蓝图被调用两次；--> 过滤 opcode=Call 精确得到 2 个构造 Call 节点
		prog := parsePython(t, `
class Point:
    def __init__(self, x, y):
        self.x = x
        self.y = y
p1 = Point(1, 2)
p2 = Point(3, 4)
`)
		res, err := prog.SyntaxFlowWithError(`Point --> * as $all; $all?{opcode: Call} as $calls`)
		require.NoError(t, err)
		vals := res.GetValues("calls")
		require.Len(t, vals, 2, "Point should be used by 2 constructor call sites")
		for _, v := range vals {
			require.Equal(t, "Call", v.GetOpcode())
		}
	})

	t.Run("inherited class blueprint used by call site", func(t *testing.T) {
		// Dog 被实例化一次；--> 过滤 Call，仅取构造调用（排除内部参数节点等）
		prog := parsePython(t, `
class Animal:
    def __init__(self, name):
        self.name = name

class Dog(Animal):
    def __init__(self, name, breed):
        Animal.__init__(self, name)
        self.breed = breed

d = Dog("Rex", "Husky")
`)
		res, err := prog.SyntaxFlowWithError(`Dog --> * as $all; $all?{opcode: Call} as $calls`)
		require.NoError(t, err)
		vals := res.GetValues("calls")
		// Dog 蓝图的 Call 用户包含：Dog(...) 构造调用 + Animal.__init__ 内部调用
		require.GreaterOrEqual(t, len(vals), 1, "Dog blueprint should have at least one Call user")
		for _, v := range vals {
			require.Equal(t, "Call", v.GetOpcode())
		}
	})

	t.Run("static method used by multiple call sites", func(t *testing.T) {
		// MathUtils.add 被调用两次，--> 过滤 Call 得 2 个节点
		prog := parsePython(t, `
class MathUtils:
    @staticmethod
    def add(a, b):
        return a + b
MathUtils.add(1, 2)
MathUtils.add(10, 20)
`)
		res, err := prog.SyntaxFlowWithError(`MathUtils.add --> * as $all; $all?{opcode: Call} as $calls`)
		require.NoError(t, err)
		vals := res.GetValues("calls")
		require.Len(t, vals, 2)
		for _, v := range vals {
			require.Equal(t, "Call", v.GetOpcode())
		}
	})
}
