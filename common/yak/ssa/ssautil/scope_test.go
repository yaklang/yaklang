package ssautil

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSyntaxBlock(t *testing.T) {
	test := assert.New(t)

	/*
		a = 1 // scope 1
		b = 1
		{
			a = 2  // sub-scope 1
			b := 2
		}
		a // 2 // scope 2
		b // 1
	*/

	table := NewRootVersionedTable[value]()
	table.CreateLexicalVariable("a", NewConsts("1"))
	table.CreateLexicalVariable("b", NewConsts("1"))
	test.Equal("const(1)", table.GetLatestVersion("a").String())

	end := BuildSyntaxBlock(table, func(sub *ScopedVersionedTable[value]) bool {
		sub.CreateLexicalVariable("a", NewConsts("2"))
		sub.CreateLexicalLocalVariable("b", NewConsts("2"))
		return false
	})

	test.NotNil(end)
	test.Equal("const(2)", end.GetLatestVersion("a").String())
	test.Equal("const(1)", end.GetLatestVersion("b").String())
}

func TestIfScope_If(t *testing.T) {
	/*
		a = 1
		b = 1
		if {
			a = 2
		}

		a// phi(1, 2)
		b// 1
	*/
	table := NewRootVersionedTable[value]()
	table.CreateLexicalVariable("a", NewConsts("1"))
	table.CreateLexicalVariable("b", NewConsts("1"))

	build := NewIfStmt(table)
	build.BuildItem(
		func(sub *ScopedVersionedTable[value]) {},
		func(sub *ScopedVersionedTable[value]) *ScopedVersionedTable[value] {
			return BuildSyntaxBlock(sub, func(sub *ScopedVersionedTable[value]) bool {
				sub.CreateLexicalVariable("a", NewConsts("2"))
				return false
			})
		},
	)
	end := build.BuildFinish(GeneratePhi)

	test := assert.New(t)
	test.Equal("phi[const(2) const(1)]", end.GetLatestVersion("a").String())
	test.Equal("const(1)", end.GetLatestVersion("b").String())
}

func TestIfScope_IfELse(t *testing.T) {
	/*
		a = 1
		b = 1
		c = 1
		if {
			a = 2
			b = 2
			c := 2
		}else {
			a = 3
		}

		a// phi(2, 3)
		b// phi(2, 1)
		c// 1
	*/
	table := NewRootVersionedTable[value]()
	table.CreateLexicalVariable("a", NewConsts("1"))
	table.CreateLexicalVariable("b", NewConsts("1"))
	table.CreateLexicalVariable("c", NewConsts("1"))

	build := NewIfStmt(table)
	build.BuildItem(
		func(sub *ScopedVersionedTable[value]) {},
		func(sub *ScopedVersionedTable[value]) *ScopedVersionedTable[value] {
			return BuildSyntaxBlock(sub, func(sub *ScopedVersionedTable[value]) bool {
				sub.CreateLexicalVariable("a", NewConsts("2"))
				sub.CreateLexicalVariable("b", NewConsts("2"))
				sub.CreateLexicalLocalVariable("c", NewConsts("2"))
				return false
			})
		},
	)
	build.BuildElse(func(sub *ScopedVersionedTable[value]) *ScopedVersionedTable[value] {
		return BuildSyntaxBlock(sub, func(sub *ScopedVersionedTable[value]) bool {
			sub.CreateLexicalVariable("a", NewConsts("3"))
			return false
		})
	})
	end := build.BuildFinish(GeneratePhi)

	test := assert.New(t)
	test.Equal("phi[const(2) const(3)]", end.GetLatestVersion("a").String())
	test.Equal("phi[const(2) const(1)]", end.GetLatestVersion("b").String())
	test.Equal("const(1)", end.GetLatestVersion("c").String())
}

func TestIfScope_IfELseIf(t *testing.T) {
	/*
		a = 1
		if c {
			a = 2
		}else if {
			a = 3
		}

		a // phi(1, 2, 3)
	*/

	global := NewRootVersionedTable[value]()
	global.CreateLexicalVariable("a", NewConsts("1"))

	build := NewIfStmt(global)
	build.BuildItem(
		func(svt *ScopedVersionedTable[value]) {},
		func(svt *ScopedVersionedTable[value]) *ScopedVersionedTable[value] {
			end := BuildSyntaxBlock(svt, func(svt *ScopedVersionedTable[value]) bool {
				svt.CreateLexicalVariable("a", NewConsts("2"))
				return false
			})
			return end
		},
	)
	build.BuildItem(
		func(svt *ScopedVersionedTable[value]) {},
		func(svt *ScopedVersionedTable[value]) *ScopedVersionedTable[value] {
			end := BuildSyntaxBlock(svt, func(svt *ScopedVersionedTable[value]) bool {
				svt.CreateLexicalVariable("a", NewConsts("3"))
				return false
			})
			return end
		},
	)
	end := build.BuildFinish(GeneratePhi)

	globalVariable := end.GetLatestVersion("a")
	test := assert.New(t)
	test.Equal("phi[const(2) const(3) const(1)]", globalVariable.String())
}

func TestIfScope_If_condition_assign(t *testing.T) {
	/*
		// this not yaklang syntax, this golang if-condition-assign syntax
			if a = 1; a == 1 {
				a = 2
			}else if a == 2 {
				a = 3
			}
			a // nil undefine
	*/

	global := NewRootVersionedTable[value]()
	one := NewConsts("1")
	two := NewConsts("2")
	// global.CreateLexicalVariable("a", one)

	var conditionVariable1, conditionVariable2 value
	build := NewIfStmt(global)
	build.BuildItem(
		func(condition *ScopedVersionedTable[value]) {
			condition.CreateLexicalVariable("a", one)
			conditionVariable1 = condition.GetLatestVersion("a")
		},
		func(svt *ScopedVersionedTable[value]) *ScopedVersionedTable[value] {
			return BuildSyntaxBlock(svt, func(svt *ScopedVersionedTable[value]) bool {
				svt.CreateLexicalVariable("a", two)
				return false
			})
		},
	)
	build.BuildItem(
		func(condition *ScopedVersionedTable[value]) {
			conditionVariable2 = condition.GetLatestVersion("a")
		},
		func(svt *ScopedVersionedTable[value]) *ScopedVersionedTable[value] {
			return BuildSyntaxBlock(svt, func(svt *ScopedVersionedTable[value]) bool {
				svt.CreateLexicalVariable("a", NewConsts("3"))
				return false
			})
		},
	)
	end := build.BuildFinish(GeneratePhi)

	globalVariable := end.GetLatestVersion("a")

	test := assert.New(t)
	test.Nil(globalVariable)
	test.Equal("const(1)", conditionVariable1.String())
	test.Equal("const(1)", conditionVariable2.String())
}

func TestIfScope_If_condition_assign_checkMerge(t *testing.T) {
	/*
		check: ssautil.Merge function

		if this example:
			global:
				a = 1
				sub1:
					a := 2
					sub2:
						a = 3

				end: merge from sub2
					a = phi[1, 3] ???  1
					a should be 1 , not phi
	*/

	global := NewRootVersionedTable[value]()
	one := NewConsts("1")
	two := NewConsts("2")
	three := NewConsts("3")

	global.CreateLexicalVariable("a", one)

	build := NewIfStmt(global)
	build.BuildItem(
		func(condition *ScopedVersionedTable[value]) {
			condition.CreateLexicalVariable("a", two)
		},
		func(svt *ScopedVersionedTable[value]) *ScopedVersionedTable[value] {
			return BuildSyntaxBlock(svt, func(svt *ScopedVersionedTable[value]) bool {
				svt.CreateLexicalVariable("a", three)
				return false
			})
		},
	)
	end := build.BuildFinish(GeneratePhi)

	test := assert.New(t)
	test.Equal("const(1)", end.GetLatestVersion("a").String())

}

func TestLoopScope(t *testing.T) {
	/*
		{
			i = 1
			for i < 10 { // phi(1, 2)
				i = 2 // phi
			}
			i // phi
		}
	*/
	test := assert.New(t)

	global := NewRootVersionedTable[value]()

	global.CreateLexicalVariable("i", NewConsts("1"))
	var conditionVariableI, bodyVariableI value
	NewLoopStmt(global, NewPhiValue).
		SetCondition(func(sub *ScopedVersionedTable[value]) {
			conditionVariableI = sub.GetLatestVersion("i")
		}).
		SetBody(func(sub *ScopedVersionedTable[value]) {
			bodyVariableI = sub.GetLatestVersion("i")
			sub.CreateLexicalVariable("i", NewConsts("2"))
		}).
		Build(SpinHandler)
	test.Equal("phi[const(1) const(2)]", conditionVariableI.String())
	test.Equal("phi[const(1) const(2)]", bodyVariableI.String())
	test.Equal("phi[const(1) const(2)]", global.GetLatestVersion("i").String())
}

func TestLoopScope_Spin(t *testing.T) {
	/*
		i = 0
		for i < 10 { // t2
			i = i + 1
			// t2 = phi(t1, 0)
			// t1 = t2 + 1
		}
		i // t2
	*/

	test := assert.New(t)

	global := NewRootVersionedTable[value]()
	zero := NewConsts("0")
	one := NewConsts("1")
	global.CreateLexicalVariable("i", zero)

	var conditionVariablePhi, bodyVariablePhi, bodyVariableBinary value
	NewLoopStmt(global, NewPhiValue).
		SetCondition(func(sub *ScopedVersionedTable[value]) {
			conditionVariablePhi = sub.GetLatestVersion("i")
		}).
		SetBody(func(body *ScopedVersionedTable[value]) {
			bodyVariablePhi = body.GetLatestVersion("i")
			test.Equal("phi[]", bodyVariablePhi.String())

			bin := NewBinary(bodyVariablePhi, one)
			test.Equal("binary(phi[], const(1))", bin.String())
			body.CreateLexicalVariable("i", bin)

			bodyVariableBinary = body.GetLatestVersion("i")
			test.Equal("binary(phi[], const(1))", bodyVariableBinary.String())
		}).
		Build(SpinHandler)

	test.Equal(*NewPhi(zero, bodyVariableBinary), *bodyVariablePhi.(*phi))
	test.Equal(bodyVariablePhi, conditionVariablePhi)
	test.Equal(*NewBinary(bodyVariablePhi, one), *bodyVariableBinary.(*binary))
}

func TestLoopScope_Spin_First(t *testing.T) {
	/*
		for i=1; i<10; i++ {
			i // phi[1, i+1]
		}
		i // undefine
	*/
	global := NewRootVersionedTable[value]()
	one := NewConsts("1")

	var thirdVariable, thirdVariableBinary, conditionVariable, bodyVariable value

	NewLoopStmt(global, NewPhiValue).
		SetFirst(func(sub *ScopedVersionedTable[value]) {
			sub.CreateLexicalVariable("i", one)
		}).
		SetCondition(func(sub *ScopedVersionedTable[value]) {
			conditionVariable = sub.GetLatestVersion("i")
		}).
		SetThird(func(sub *ScopedVersionedTable[value]) {
			thirdVariable = sub.GetLatestVersion("i")
			thirdVariableBinary = NewBinary(thirdVariable, one)
			sub.CreateLexicalVariable("i", thirdVariableBinary)
		}).
		SetBody(func(sub *ScopedVersionedTable[value]) {
			bodyVariable = sub.GetLatestVersion("i")
		}).
		Build(SpinHandler)

	globalVariable := global.GetLatestVersion("i")
	test := assert.New(t)

	test.Equal(*NewPhi(one, thirdVariableBinary), *bodyVariable.(*phi))
	test.Nil(globalVariable)
	test.Equal(*NewBinary(thirdVariable, one), *thirdVariableBinary.(*binary))
	test.Equal(conditionVariable, thirdVariable)
	test.Equal(bodyVariable, thirdVariable)

}

func TestIfLoopScope_Basic(t *testing.T) {
	/*
		i = 0
		for i < 10 { //  phi[0, phi[i+1, i+2]]
			if i == 0 {
				i = i + 1
			}else {
				i = i + 2
			}
			i // phi[i+1, i+2]
		}
		i // phi[0, phi[i+1, i+2]]
	*/

	test := assert.New(t)

	global := NewRootVersionedTable[value]()
	zero := NewConsts("0")
	one := NewConsts("1")
	two := NewConsts("2")
	global.CreateLexicalVariable("i", zero)

	var conditionVariable, bodyVariablePhi, trueVariableBinary, falseVariableBinary, globalVariablePhi value
	NewLoopStmt(global, NewPhiValue).
		SetCondition(func(sub *ScopedVersionedTable[value]) {
			conditionVariable = sub.GetLatestVersion("i")
		}).
		SetBody(func(body *ScopedVersionedTable[value]) {
			build := NewIfStmt(body)
			build.BuildItem(
				func(*ScopedVersionedTable[value]) {},
				func(body *ScopedVersionedTable[value]) {
					trueVariablePhi := body.GetLatestVersion("i")
					body.CreateLexicalVariable("i", NewBinary(trueVariablePhi, one))
					trueVariableBinary = body.GetLatestVersion("i")
				},
			)
			build.BuildElse(func(body *ScopedVersionedTable[value]) {
				falseVariablePhi := body.GetLatestVersion("i")
				body.CreateLexicalVariable("i", NewBinary(falseVariablePhi, two))
				falseVariableBinary = body.GetLatestVersion("i")
			})
			build.BuildFinish(GeneratePhi)
			bodyVariablePhi = body.GetLatestVersion("i")
		}).
		Build(SpinHandler)

	globalVariablePhi = global.GetLatestVersion("i")

	test.Equal(*NewPhi(trueVariableBinary, falseVariableBinary), *bodyVariablePhi.(*phi))
	test.Equal(*NewPhi(zero, bodyVariablePhi), *globalVariablePhi.(*phi))
	test.Equal(conditionVariable, globalVariablePhi)
}
