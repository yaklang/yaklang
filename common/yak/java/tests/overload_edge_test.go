package tests

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestMethodOverloadDispatchEdgeCases(t *testing.T) {
	t.Run("dispatch by boolean and int literal", func(t *testing.T) {
		code := `
class A {
	int value;
	void set(int num) {
		this.value = 10;
	}
	void set(boolean b) {
		this.value = 20;
	}
	int get() {
		return this.value;
	}
}
class Main {
	void main() {
		A a = new A();
		a.set(true);
		print(a.get());
		a.set(1);
		print(a.get());
	}
}
`
		ssatest.CheckSyntaxFlow(t, code,
			`print(* #-> * as $target)`,
			map[string][]string{
				"target": {"10", "20"},
			},
			ssaapi.WithLanguage(ssaconfig.JAVA),
		)
	})

	t.Run("dispatch still correct through alias reference", func(t *testing.T) {
		code := `
class A {
	int value;
	void set(int left) {
		this.value = 31;
	}
	void set(int left, int right) {
		this.value = 32;
	}
	int get() {
		return this.value;
	}
}
class Main {
	void main() {
		A a = new A();
		A ref = a;
		ref.set(1, 2);
		print(a.get());
		ref.set(1);
		print(a.get());
	}
}
`
		ssatest.CheckSyntaxFlow(t, code,
			`print(* #-> * as $target)`,
			map[string][]string{
				"target": {"31", "32"},
			},
			ssaapi.WithLanguage(ssaconfig.JAVA),
		)
	})

	t.Run("dispatch by empty parameter list and one parameter", func(t *testing.T) {
		code := `
class A {
	int value;
	void set() {
		this.value = 41;
	}
	void set(int left) {
		this.value = 42;
	}
	int get() {
		return this.value;
	}
}
class Main {
	void main() {
		A a = new A();
		a.set();
		print(a.get());
		a.set(1);
		print(a.get());
	}
}
`
		ssatest.CheckSyntaxFlow(t, code,
			`print(* #-> * as $target)`,
			map[string][]string{
				"target": {"41", "42"},
			},
			ssaapi.WithLanguage(ssaconfig.JAVA),
		)
	})

	t.Run("dispatch with variadic candidate", func(t *testing.T) {
		code := `
class A {
	int value;
	void set(int first, int... rest) {
		this.value = 61;
	}
	void set(String raw) {
		this.value = 62;
	}
	int get() {
		return this.value;
	}
}
class Main {
	void main() {
		A a = new A();
		a.set(1);
		print(a.get());
		a.set("x");
		print(a.get());
	}
}
`
		ssatest.CheckSyntaxFlow(t, code,
			`print(* #-> * as $target)`,
			map[string][]string{
				"target": {"61", "62"},
			},
			ssaapi.WithLanguage(ssaconfig.JAVA),
		)
	})

	t.Run("dispatch from parent class overload", func(t *testing.T) {
		code := `
class Base {
	int value;
	void set(String raw) {
		this.value = 71;
	}
	int get() {
		return this.value;
	}
}
class A extends Base {
	void set(int left) {
		this.value = 72;
	}
}
class Main {
	void main() {
		A a = new A();
		a.set("x");
		print(a.get());
		a.set(1);
		print(a.get());
	}
}
`
		ssatest.CheckSyntaxFlow(t, code,
			`print(* #-> * as $target)`,
			map[string][]string{
				"target": {"71", "72"},
			},
			ssaapi.WithLanguage(ssaconfig.JAVA),
		)
	})

	t.Run("complex direct calls with alias select different overload candidates", func(t *testing.T) {
		code := `
class A {
	int value;
	void set(int num) {
		this.value = 81;
	}
	void set(String raw) {
		this.value = 82;
	}
	int get() {
		return this.value;
	}
}
class Main {
	void main() {
		A a = new A();
		A alias = a;
		alias.set(1);
		print(a.get());
		a.set("x");
		print(alias.get());
	}
}
`
		ssatest.CheckSyntaxFlow(t, code,
			`print(* #-> * as $target)`,
			map[string][]string{
				"target": {"81", "82"},
			},
			ssaapi.WithLanguage(ssaconfig.JAVA),
		)
	})

	t.Run("constructor overload through direct creation and alias chain", func(t *testing.T) {
		code := `
class Box {
	int value;
	Box(int num) {
		this.value = 91;
	}
	Box(String raw) {
		this.value = 92;
	}
	int get() {
		return this.value;
	}
}
class Main {
	void main() {
		Box first = new Box(1);
		Box alias1 = first;
		print(alias1.get());
		Box second = new Box("x");
		Box alias2 = second;
		print(alias2.get());
	}
}
`
		ssatest.CheckSyntaxFlow(t, code,
			`print(* #-> * as $target)`,
			map[string][]string{
				"target": {"91", "92"},
			},
			ssaapi.WithLanguage(ssaconfig.JAVA),
		)
	})
}
