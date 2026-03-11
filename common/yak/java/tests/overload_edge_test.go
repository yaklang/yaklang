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
}
