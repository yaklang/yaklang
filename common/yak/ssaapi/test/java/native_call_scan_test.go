package java

import (
	"github.com/yaklang/yaklang/common/consts"
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestScanWithIfStatement(t *testing.T) {
	code := `package com.example.A;
	public class A {
		public static void main(String[] args) {
			bb1;
			a = 1;
			if (c){
				a = 2;
			}else {
				a = 3;
			}
			bb2;
		}
	}
	`
	ssatest.CheckSyntaxFlow(t, code, `bb2<scanPrevious> as $target1;bb1<scanNext> as $target2`,
		map[string][]string{
			"target1": {"1", "2", "3", "Undefined-bb1", "Undefined-bb2", "Undefined-c"},
			"target2": {"1", "2", "3", "Undefined-bb1", "Undefined-bb2", "Undefined-c"},
		},
		ssaapi.WithRawLanguage("java"))
}

func TestScanWithForStatement(t *testing.T) {
	code := `package com.example.A;
	public class A {
		public static void main(String[] args) {
			bb1;
			for (int i = num; i < 10; i++) {
				a += i;
			}
			bb2;
		}
	}
	`

	ssatest.CheckSyntaxFlow(t, code, `bb2<scanPrevious> as $target1;bb1<scanNext> as $target2`,
		map[string][]string{
			"target1": {"1", "10", "Undefined-a", "Undefined-bb1", "Undefined-bb2", "Undefined-i", "add(Undefined-a, phi(i)[Undefined-i,add(i, 1)])", "add(phi(i)[Undefined-i,add(i, 1)], 1)", "lt(phi(i)[Undefined-i,add(i, 1)], 10)"},
			"target2": {"1", "10", "Undefined-a", "Undefined-bb1", "Undefined-bb2", "Undefined-i", "add(Undefined-a, phi(i)[Undefined-i,add(i, 1)])", "add(phi(i)[Undefined-i,add(i, 1)], 1)", "lt(phi(i)[Undefined-i,add(i, 1)], 10)"},
		},
		ssaapi.WithRawLanguage("java"))
}

func TestScanWithSwitchStatemt(t *testing.T) {
	code := `package com.example.A;
	public class A {
		public static void main(String[] args) {
			bb1;
			a= 0;
			switch (c){
			case 1:
				a=11;
			case 2:
				a=22;
			case 3:
				a=33;
			default:
				a=44;
			}
			bb2;
		}
	}
	`
	ssatest.CheckSyntaxFlow(t, code, `bb2<scanPrevious> as $target1;bb1<scanNext> as $target2`,
		map[string][]string{
			"target1": {"0", "1", "11", "2", "22", "3", "33", "44", "Undefined-bb1", "Undefined-bb2", "Undefined-c"},
			"target2": {"0", "1", "11", "2", "22", "3", "33", "44", "Undefined-bb1", "Undefined-bb2", "Undefined-c"},
		},
		ssaapi.WithRawLanguage("java"))
}

func TestScanPreviousIfStmtWithConfig(t *testing.T) {
	code := `package com.example.A;
	public class A {
		public static void main(String[] args) {
			bb1;
			a = 1;
			if (c){
				a = 2;
			}else {
				a = 3;
			}
			bb2;
		}
	}
	`
	t.Run("test exclude", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, code, "bb2<scanPrevious(exclude=`*?{opcode: const}`)> as $result;",
			map[string][]string{
				"result": {"Undefined-bb1", "Undefined-bb2", "Undefined-c"},
			}, ssaapi.WithLanguage(consts.JAVA))
	})
	t.Run("test include", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, code, "bb2<scanPrevious{include:`* ?{opcode:const}`}> as $result;",
			map[string][]string{
				"result": {"1", "2", "3"},
			}, ssaapi.WithLanguage(consts.JAVA))
	})

	t.Run("test until", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, code, "bb2<scanPrevious(until=`*?{opcode: const}`)> as $result;",
			map[string][]string{
				"result": {"Undefined-bb2"},
			}, ssaapi.WithLanguage(consts.JAVA))

	})

	t.Run("test hook", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, code, "bb2<scanPrevious(hook=`*?{opcode: const} as $num`)> as $result;",
			map[string][]string{
				"result": {"1", "2", "3", "Undefined-bb1", "Undefined-bb2", "Undefined-c"},
				"num":    {"1", "2", "3"},
			}, ssaapi.WithLanguage(consts.JAVA))

	})
}

func TestScanNextLoopWithConfig(t *testing.T) {
	code := `package com.example.A;
	public class A {
		public static void main(String[] args) {
			bb1;
			for (int i = 0; i < 10; i++) {
				a += i;
			}
			bb2;
		}
	}`

	t.Run("test exclude", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, code, "bb1<scanNext(exclude=`*?{opcode: const}`)> as $result;",
			map[string][]string{
				"result": {"Undefined-a", "Undefined-bb1", "Undefined-bb2", "add(Undefined-a, phi(i)[0,add(i, 1)])", "add(phi(i)[0,add(i, 1)], 1)", "lt(phi(i)[0,add(i, 1)], 10)"},
			}, ssaapi.WithLanguage(consts.JAVA))
	})
	t.Run("test include", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, code, "bb1<scanNext{include:`* ?{opcode:const}`}> as $result;",
			map[string][]string{
				"result": {"0", "0", "1", "10"},
			}, ssaapi.WithLanguage(consts.JAVA))
	})

	t.Run("test until", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, code, "bb1<scanNext(until=`*?{opcode: const}`)> as $result;",
			map[string][]string{
				"result": {"Undefined-bb1"},
			}, ssaapi.WithLanguage(consts.JAVA))
	})

	t.Run("test hook", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, code, "bb1<scanNext(hook=`*?{opcode: const} as $num`)> as $result;",
			map[string][]string{
				"result": {"0", "0", "1", "10", "Undefined-a", "Undefined-bb1", "Undefined-bb2", "add(Undefined-a, phi(i)[0,add(i, 1)])", "add(phi(i)[0,add(i, 1)], 1)", "lt(phi(i)[0,add(i, 1)], 10)"},
				"num":    {"0", "0", "1", "10"},
			}, ssaapi.WithLanguage(consts.JAVA))

	})
}
