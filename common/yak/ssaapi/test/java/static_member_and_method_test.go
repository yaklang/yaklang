package java

import (
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
	"testing"
)

func Test_Static_Member_And_Method(t *testing.T) {
	t.Run("test simple static member", func(t *testing.T) {
		code := `
		class A {
		public static int a = 1;
		}
	class B {
		public static void main(String[] args) {
			System.out.println(A.a);
		}
	}
	`
		ssatest.CheckSyntaxFlow(t, code, `
		System.out.println(* #-> as $a)
		`, map[string][]string{
			"a": {"1"},
		}, ssaapi.WithLanguage(ssaapi.JAVA),
		)
	})
	t.Run("test call self static method", func(t *testing.T) {
		code := `class A {
		public static int get() {
			return 	 22;
		}
		public static void main(String[] args) {
			System.out.println(A.get());
		}
	}`

		ssatest.CheckSyntaxFlow(t, code, `
		System.out.println(* #-> as $a)
		`, map[string][]string{
			"a": {"22"},
		}, ssaapi.WithLanguage(ssaapi.JAVA),
		)
	})

	t.Run("test call self normal method", func(t *testing.T) {
		code := `class A {
		public int get() {
			return 	 22;
		}
		public static void main(String[] args) {
			A a = new A();
			System.out.println(a.get());
		}
	}`
		ssatest.CheckSyntaxFlow(t, code, `
		System.out.println(* #-> as $a)
		`, map[string][]string{
			"a": {"22"},
		}, ssaapi.WithLanguage(ssaapi.JAVA),
		)
	})

	t.Run("test the static member and method of the same name without instantiation ", func(t *testing.T) {
		code := `
		class A {
		public static int get = 11;
		public static int get() {
			return 	 22;
		}
	}
		class B {
		public static void main(String[] args) {
			A a = new A();
			System.out.println(a.get());
			System.out.println(a.get);
			
		}
	}
`
		ssatest.CheckSyntaxFlow(t, code, `
		System.out.println(* #-> as $a)
		`, map[string][]string{
			"a": {"11", "22"},
		}, ssaapi.WithLanguage(ssaapi.JAVA),
		)
	})

	t.Run("test the static member and method of the same name ", func(t *testing.T) {
		code := `class A {
		public static int get = 11;
		public static int get() {
			return 	 22;
		}
	}
		class B {
		public static void main(String[] args) {
			System.out.println(A.get());
			System.out.println(A.get);
			
		}
	}
`
		ssatest.CheckSyntaxFlow(t, code, `
		System.out.println(* #-> as $a)
		`, map[string][]string{
			"a": {"11", "22"},
		}, ssaapi.WithLanguage(ssaapi.JAVA),
		)
	})

	t.Run("test static method ", func(t *testing.T) {
		code := `class A {
		public static int get() {
			return 	 1;
		}
		}
		class B {
		public static void main(String[] args) {
			A a = new A();
			System.out.println(a.get());
		}
		}
	`
		ssatest.CheckSyntaxFlow(t, code, `
		System.out.println(* #-> as $a)
		`, map[string][]string{
			"a": {"1"},
		}, ssaapi.WithLanguage(ssaapi.JAVA),
		)
	})

	t.Run("test static method without instantiation", func(t *testing.T) {
		code := `class A {
		public static int get() {
			return 	 1;
		}
}
		class B {
		public static void main(String[] args) {
			System.out.println(A.get());
		}
	
	}`
		ssatest.CheckSyntaxFlow(t, code, `
		System.out.println(* #-> as $a)
		`, map[string][]string{
			"a": {"1"},
		}, ssaapi.WithLanguage(ssaapi.JAVA),
		)
	})

}

func Test_Static_Member_And_Method_Cross_File(t *testing.T) {
	t.Run("test static method ", func(t *testing.T) {
		vf := filesys.NewVirtualFs()
		vf.AddFile("src/main/java/A.java", `
	package A; 
	class A {
		public static int get() {
			return 	 1;
		}
	}
	`)
		vf.AddFile("src/main/java/B.java", `
	package B; 
	import A.A;
	class B {
		public static void main(String[] args) {
			A a = new A();
			System.out.println(a.get());
		}
	}
	`)

		ssatest.CheckSyntaxFlowWithFS(t, vf, `
		System.out.println(* #-> as $a)
		`, map[string][]string{
			"a": {"1"},
		}, false, ssaapi.WithLanguage(ssaapi.JAVA),
		)
	})
	t.Run("test static method without instantiation", func(t *testing.T) {
		vf := filesys.NewVirtualFs()
		vf.AddFile("src/main/java/A.java", `
	package A; 
	class A {
		public static int get() {
			return 	 1;
		}
	}
	`)
		vf.AddFile("src/main/java/B.java", `
	package B; 
	import A.A;
	class B {
		public static void main(String[] args) {
			System.out.println(A.get());
		}
	}
	`)

		ssatest.CheckSyntaxFlowWithFS(t, vf, `
		System.out.println(* #-> as $a)
		`, map[string][]string{
			"a": {"1"},
		}, false, ssaapi.WithLanguage(ssaapi.JAVA),
		)
	})
	t.Run("test static member ", func(t *testing.T) {
		vf := filesys.NewVirtualFs()
		vf.AddFile("src/main/java/A.java", `
	package A; 
	class A {
		public static int a = 11;
	}
	`)
		vf.AddFile("src/main/java/B.java", `
	package B; 
	import A.A;
	class B {
		public static void main(String[] args) {
			
			System.out.println(A.a);
		}
	}
	`)

		ssatest.CheckSyntaxFlowWithFS(t, vf, `
		System.out.println(* #-> as $a)
		`, map[string][]string{
			"a": {"11"},
		}, false, ssaapi.WithLanguage(ssaapi.JAVA),
		)
	})
	t.Run("test static member with instantiation ", func(t *testing.T) {
		vf := filesys.NewVirtualFs()
		vf.AddFile("src/main/java/A.java", `
	package A; 
	class A {
		public static int a = 11;
	}
	`)
		vf.AddFile("src/main/java/B.java", `
	package B; 
	import A.A;
	class B {
		public static void main(String[] args) {
			A a = new A();
			System.out.println(a.a);
		}
	}
	`)

		ssatest.CheckSyntaxFlowWithFS(t, vf, `
		System.out.println(* #-> as $a)
		`, map[string][]string{
			"a": {"11"},
		}, false, ssaapi.WithLanguage(ssaapi.JAVA),
		)
	})
	t.Run("test the static member and method of the same name without instantiation ", func(t *testing.T) {
		vf := filesys.NewVirtualFs()
		vf.AddFile("src/main/java/A.java", `
	package A; 
	class A {
		public static int get = 11;
		public static int get() {
			return 	 22;
		}
	}
	`)
		vf.AddFile("src/main/java/B.java", `
	package B; 
	import A.A;
	class B {
		public static void main(String[] args) {
			A a = new A();
			System.out.println(a.get());
			System.out.println(a.get);
			
		}
	}
	`)

		ssatest.CheckSyntaxFlowWithFS(t, vf, `
		System.out.println(* #-> as $a)
		`, map[string][]string{
			"a": {"11", "22"},
		}, false, ssaapi.WithLanguage(ssaapi.JAVA),
		)
	})
	t.Run("test the static member and method of the same name ", func(t *testing.T) {
		vf := filesys.NewVirtualFs()
		vf.AddFile("src/main/java/A.java", `
	package A; 
	class A {
		public static int get = 11;
		public static int get() {
			return 	 22;
		}
	}
	`)
		vf.AddFile("src/main/java/B.java", `
	package B; 
	import A.A;
	class B {
		public static void main(String[] args) {
			System.out.println(A.get());
			System.out.println(A.get);
			
		}
	}
	`)

		ssatest.CheckSyntaxFlowWithFS(t, vf, `
		System.out.println(* #-> as $a)
		`, map[string][]string{
			"a": {"11", "22"},
		}, false, ssaapi.WithLanguage(ssaapi.JAVA),
		)
	})
}
