package java

import (
	"testing"

	"github.com/stretchr/testify/assert"
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
	ssatest.Check(t, code, func(prog *ssaapi.Program) error {
		prog.Show()
		result := prog.SyntaxFlowChain("bb2<scanPrevious> as $result").Show(false)
		assert.Equal(t, 6, result.Len())
		result = prog.SyntaxFlowChain("bb1<scanNext> as $result").Show(false)
		assert.Equal(t, 6, result.Len())
		return nil
	}, ssaapi.WithLanguage(ssaapi.JAVA))
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

	ssatest.Check(t, code, func(prog *ssaapi.Program) error {
		prog.Show()
		result := prog.SyntaxFlowChain("bb2<scanPrevious> as $result").Show(false)
		assert.Equal(t, 9, result.Len())

		result = prog.SyntaxFlowChain("bb1<scanNext> as $result").Show()
		assert.Equal(t, 9, result.Len())

		return nil
	}, ssaapi.WithLanguage(ssaapi.JAVA))
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
	ssatest.Check(t, code, func(prog *ssaapi.Program) error {
		prog.Show()
		result := prog.SyntaxFlowChain("bb2<scanPrevious> as $result").Show(false)
		assert.Equal(t, 11, result.Len())

		result = prog.SyntaxFlowChain("bb1<scanNext> as $result").Show(false)
		assert.Equal(t, 11, result.Len())

		return nil
	}, ssaapi.WithLanguage(ssaapi.JAVA))
}

func TestScanPreviousWithParam(t *testing.T) {
	t.Run("test if stmt",func(t *testing.T) {
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
	ssatest.Check(t, code, func(prog *ssaapi.Program) error {
		prog.Show()
		result := prog.SyntaxFlowChain("bb2<scanPrevious(until=`*?{opcode: const}`)> as $result").Show(false)
		assert.Equal(t, 1, result.Len())

		result = prog.SyntaxFlowChain("bb2<scanPrevious(hook=`*?{opcode: const}`)> as $result").Show(false)
		assert.Equal(t, 3, result.Len())

		result = prog.SyntaxFlowChain("bb2<scanPrevious(exclude=`*?{opcode: const}`)> as $result").Show(false)
		assert.Equal(t, 3, result.Len())
		return nil
	}, ssaapi.WithLanguage(ssaapi.JAVA))
	})

	t.Run("test loop stmt",func(t *testing.T) {
		code := `package com.example.A;
	public class A {
		public static void main(String[] args) {
			bb1;
			for (int i = 0; i < 10; i++) {
				a += i;
			}
			bb2;
		}
	}
	`
	ssatest.Check(t, code, func(prog *ssaapi.Program) error {
		prog.Show()
		result := prog.SyntaxFlowChain("bb2<scanPrevious(until=`*?{opcode: const}`)>?{have:'10'} as $result").Show(false)
		assert.Equal(t, 1, result.Len())

		result = prog.SyntaxFlowChain("bb2<scanPrevious(hook=`*?{opcode: const}`)> as $result").Show(false)
		assert.Equal(t, 4, result.Len())

		result = prog.SyntaxFlowChain("bb2<scanPrevious(exclude=`*?{opcode: const}`)> as $result").Show()
		assert.Equal(t, 6, result.Len())
		return nil
	}, ssaapi.WithLanguage(ssaapi.JAVA))
	})

	t.Run("test if-else if stmt",func(t *testing.T) {
		code := `package com.example.A;
	public class A {
		public void sink(String input){
			bb1;
			println();
			if (cond1){
				println1();
			}else if(cond2){
			    println2();
			}else{
				println3();
			}
			bb2;
		}
	}
	`
	ssatest.Check(t, code, func(prog *ssaapi.Program) error {
		prog.Show()
		result := prog.SyntaxFlowChain("bb2<scanPrevious(until=`<fullTypeName>?{have:'println2'}`)> as $result").Show(false)
		assert.Equal(t,result.Len(), 1)
		result = prog.SyntaxFlowChain("bb1<scanNext(until=`<fullTypeName>?{have:'println2'}`)> as $result").Show(false)
		assert.Equal(t,result.Len(), 1)
		result=prog.SyntaxFlowChain("bb1<scanNext(hook=`<fullTypeName>?{have:'println'}`)> as $result").Show(true)
		assert.Equal(t,result.Len(), 4)
		return nil
	}, ssaapi.WithLanguage(ssaapi.JAVA))
	})
}
