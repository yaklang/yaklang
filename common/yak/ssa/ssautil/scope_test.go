package ssautil

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

type BuilderReturnScopeFunc func(*ScopedVersionedTable[value]) *ScopedVersionedTable[value]
type BuilderFunc func(*ScopedVersionedTable[value])

var (
	zero = NewConsts("0")
	one  = NewConsts("1")
	two  = NewConsts("2")
)

func TestSyntaxBlock(t *testing.T) {

	check := func(
		beforeBlock BuilderFunc,
		block BuilderReturnScopeFunc,
		afterBlock BuilderFunc,
	) {
		global := NewRootVersionedTable[value](NewVersioned[value])
		/*
			beforeBlock()
			{
				block()
			}
			afterBlock()
		*/

		beforeBlock(global)
		end := BuildSyntaxBlock(global, func(svt *ScopedVersionedTable[value]) *ScopedVersionedTable[value] {
			return block(svt)
		})
		afterBlock(end)
	}

	t.Run("test scope cover", func(t *testing.T) {
		/*
			a = 1
			{
				a = 2
			}
			a // 2
		*/
		test := assert.New(t)
		check(
			func(svt *ScopedVersionedTable[value]) {
				svt.WriteVariable("a", one)
			},
			func(svt *ScopedVersionedTable[value]) *ScopedVersionedTable[value] {
				svt.WriteVariable("a", two)
				return svt
			},
			func(svt *ScopedVersionedTable[value]) {
				test.Equal(two, svt.ReadValue("a"))
			},
		)
	})
	t.Run("test scope local variable, not cover", func(t *testing.T) {
		/*
			a = 1
			{
				a := 2
			}
			a //
		*/
		test := assert.New(t)
		check(
			func(svt *ScopedVersionedTable[value]) {
				svt.WriteVariable("a", one)
			},
			func(svt *ScopedVersionedTable[value]) *ScopedVersionedTable[value] {
				svt.WriteLocalVariable("a", two)
				return svt
			},
			func(svt *ScopedVersionedTable[value]) {
				test.Equal(one, svt.ReadValue("a"))
			},
		)
	})

	t.Run("test scope local variable, but not cover", func(t *testing.T) {
		test := assert.New(t)
		/*
			{
				a := 1
				{
					a = 2
				}
				a // 2
			}
			a // nil
		*/
		check(
			func(svt *ScopedVersionedTable[value]) {},
			func(svt *ScopedVersionedTable[value]) *ScopedVersionedTable[value] {
				svt.WriteVariable("a", one)
				svt = BuildSyntaxBlock(svt, func(svt *ScopedVersionedTable[value]) *ScopedVersionedTable[value] {
					svt.WriteVariable("a", two)
					return svt
				})
				test.Equal(two, svt.ReadValue("a"))
				return svt
			},
			func(svt *ScopedVersionedTable[value]) {
				test.Nil(svt.ReadValue("a"))
			},
		)
	})

	t.Run("test scope variable but not cover", func(t *testing.T) {
		/*
			{
				a = 1
			}
			a // nil
		*/
		test := assert.New(t)
		check(
			func(svt *ScopedVersionedTable[value]) {},
			func(svt *ScopedVersionedTable[value]) *ScopedVersionedTable[value] {
				svt.WriteVariable("a", one)
				return svt
			},
			func(svt *ScopedVersionedTable[value]) {
				test.Nil(svt.ReadValue("a"))
			},
		)
	})
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
	table := NewRootVersionedTable[value](NewVersioned[value])
	table.WriteVariable("a", NewConsts("1"))
	table.WriteVariable("b", NewConsts("1"))

	build := NewIfStmt(table)
	build.BuildItem(
		func(sub *ScopedVersionedTable[value]) {},
		func(sub *ScopedVersionedTable[value]) *ScopedVersionedTable[value] {
			return BuildSyntaxBlock(sub, func(sub *ScopedVersionedTable[value]) *ScopedVersionedTable[value] {
				sub.WriteVariable("a", NewConsts("2"))
				return sub
			})
		},
	)
	end := build.BuildFinish(GeneratePhi)

	test := assert.New(t)
	test.Equal("phi[const(2) const(1)]", end.ReadValue("a").String())
	test.Equal("const(1)", end.ReadValue("b").String())
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
	table := NewRootVersionedTable[value](NewVersioned[value])
	table.WriteVariable("a", NewConsts("1"))
	table.WriteVariable("b", NewConsts("1"))
	table.WriteVariable("c", NewConsts("1"))

	build := NewIfStmt(table)
	build.BuildItem(
		func(sub *ScopedVersionedTable[value]) {},
		func(sub *ScopedVersionedTable[value]) *ScopedVersionedTable[value] {
			return BuildSyntaxBlock(sub, func(sub *ScopedVersionedTable[value]) *ScopedVersionedTable[value] {
				sub.WriteVariable("a", NewConsts("2"))
				sub.WriteVariable("b", NewConsts("2"))
				sub.WriteLocalVariable("c", NewConsts("2"))
				return sub
			})
		},
	)
	build.BuildElse(func(sub *ScopedVersionedTable[value]) *ScopedVersionedTable[value] {
		return BuildSyntaxBlock(sub, func(sub *ScopedVersionedTable[value]) *ScopedVersionedTable[value] {
			sub.WriteVariable("a", NewConsts("3"))
			return sub
		})
	})
	end := build.BuildFinish(GeneratePhi)

	test := assert.New(t)
	test.Equal("phi[const(2) const(3)]", end.ReadValue("a").String())
	test.Equal("phi[const(2) const(1)]", end.ReadValue("b").String())
	test.Equal("const(1)", end.ReadValue("c").String())
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

	global := NewRootVersionedTable[value](NewVersioned[value])
	global.WriteVariable("a", NewConsts("1"))

	build := NewIfStmt(global)
	build.BuildItem(
		func(svt *ScopedVersionedTable[value]) {},
		func(svt *ScopedVersionedTable[value]) *ScopedVersionedTable[value] {
			end := BuildSyntaxBlock(svt, func(sub *ScopedVersionedTable[value]) *ScopedVersionedTable[value] {
				sub.WriteVariable("a", NewConsts("2"))
				return sub
			})
			return end
		},
	)
	build.BuildItem(
		func(svt *ScopedVersionedTable[value]) {},
		func(svt *ScopedVersionedTable[value]) *ScopedVersionedTable[value] {
			end := BuildSyntaxBlock(svt, func(svt *ScopedVersionedTable[value]) *ScopedVersionedTable[value] {
				svt.WriteVariable("a", NewConsts("3"))
				return svt
			})
			return end
		},
	)
	end := build.BuildFinish(GeneratePhi)

	globalVariable := end.ReadValue("a")
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

	global := NewRootVersionedTable[value](NewVersioned[value])
	one := NewConsts("1")
	two := NewConsts("2")
	// global.WriteVariable("a", one)

	var conditionVariable1, conditionVariable2 value
	build := NewIfStmt(global)
	build.BuildItem(
		func(condition *ScopedVersionedTable[value]) {
			condition.WriteVariable("a", one)
			conditionVariable1 = condition.ReadValue("a")
		},
		func(svt *ScopedVersionedTable[value]) *ScopedVersionedTable[value] {
			return BuildSyntaxBlock(svt, func(svt *ScopedVersionedTable[value]) *ScopedVersionedTable[value] {
				svt.WriteVariable("a", two)
				return svt
			})
		},
	)
	build.BuildItem(
		func(condition *ScopedVersionedTable[value]) {
			conditionVariable2 = condition.ReadValue("a")
		},
		func(svt *ScopedVersionedTable[value]) *ScopedVersionedTable[value] {
			return BuildSyntaxBlock(svt, func(svt *ScopedVersionedTable[value]) *ScopedVersionedTable[value] {
				svt.WriteVariable("a", NewConsts("3"))
				return svt
			})
		},
	)
	end := build.BuildFinish(GeneratePhi)

	globalVariable := end.ReadValue("a")

	test := assert.New(t)
	test.Nil(globalVariable)
	test.Equal("const(1)", conditionVariable1.String())
	test.Equal("const(1)", conditionVariable2.String())
}

func TestIfScope_If_condition_assign_checkMerge(t *testing.T) {
	/*
		check: ssautil.Merge function

		a = 1
		if a = 2 {
			a = 3
		}
		a // should 1, not phi
	*/
	check := func(
		beforeIf BuilderFunc,
		buildCondition BuilderFunc,
		buildBody BuilderReturnScopeFunc,
		afterIf BuilderFunc,
	) {
		global := NewRootVersionedTable[value](NewVersioned[value])
		beforeIf(global)
		build := NewIfStmt(global)
		build.BuildItem(
			buildCondition, buildBody,
		)
		end := build.BuildFinish(GeneratePhi)
		afterIf(end)
	}

	t.Run("no local variable", func(t *testing.T) {
		/*
			a = 1
			if a = 2 {
				a = 3
			}
			a // phi(1, 3)
		*/

		test := assert.New(t)
		check(
			func(svt *ScopedVersionedTable[value]) {
				svt.WriteVariable("a", NewConsts("1"))
			},
			func(svt *ScopedVersionedTable[value]) {
				svt.WriteVariable("a", NewConsts("2"))
			},
			func(svt *ScopedVersionedTable[value]) *ScopedVersionedTable[value] {
				return BuildSyntaxBlock(svt, func(svt *ScopedVersionedTable[value]) *ScopedVersionedTable[value] {
					svt.WriteVariable("a", NewConsts("3"))
					return svt
				})
			},
			func(svt *ScopedVersionedTable[value]) {
				test.Equal("phi[const(3) const(1)]", svt.ReadValue("a").String())
			},
		)
	})

	t.Run("local variable", func(t *testing.T) {
		/*
			a = 1
			if a := 2 {
				a = 3
			}
			a // 1
		*/
		test := assert.New(t)
		check(
			func(svt *ScopedVersionedTable[value]) {
				svt.WriteVariable("a", NewConsts("1"))
			},
			func(svt *ScopedVersionedTable[value]) {
				svt.WriteLocalVariable("a", NewConsts("2"))
			},
			func(svt *ScopedVersionedTable[value]) *ScopedVersionedTable[value] {
				return BuildSyntaxBlock(svt, func(svt *ScopedVersionedTable[value]) *ScopedVersionedTable[value] {
					svt.WriteVariable("a", NewConsts("3"))
					return svt
				})
			},
			func(svt *ScopedVersionedTable[value]) {
				test.Equal("const(1)", svt.ReadValue("a").String())
			},
		)
	})
}

func TestIfScope_In_SyntaxBlock(t *testing.T) {
	type builderFunc func(*ScopedVersionedTable[value])
	check := func(
		beforeBlock builderFunc,
		beforeIf builderFunc,
		buildIfBody builderFunc,
		afterIf builderFunc,
		afterBlock builderFunc,
	) {
		/*
			beforeBlock()
			{
				beforeIf()
				if c {
					buildIfBody()
				}
				afterIf()
			}
			afterBlock()
		*/

		global := NewRootVersionedTable[value](NewVersioned[value])
		beforeBlock(global)
		end := BuildSyntaxBlock(global, func(svt *ScopedVersionedTable[value]) *ScopedVersionedTable[value] {
			beforeIf(svt)
			builder := NewIfStmt(svt)
			builder.BuildItem(
				func(svt *ScopedVersionedTable[value]) {},
				func(svt *ScopedVersionedTable[value]) *ScopedVersionedTable[value] {
					return BuildSyntaxBlock(svt, func(svt *ScopedVersionedTable[value]) *ScopedVersionedTable[value] {
						buildIfBody(svt)
						return svt
					})
				},
			)
			end := builder.BuildFinish(GeneratePhi)
			afterIf(end)
			return end
		})
		afterBlock(end)
	}
	zero := NewConsts("0")
	one := NewConsts("1")
	two := NewConsts("2")

	t.Run("test 1, if in syntax block", func(t *testing.T) {
		/*
			a = 0
			{
				a = 1
				if c {
					a = 2
				}
				a // phi[1, 2]
			}
			a  //  phi[1,2]
		*/
		test := assert.New(t)
		check(
			func(svt *ScopedVersionedTable[value]) {
				svt.WriteVariable("a", zero)
			},

			func(svt *ScopedVersionedTable[value]) {
				svt.WriteVariable("a", one)
			},
			func(svt *ScopedVersionedTable[value]) {
				svt.WriteVariable("a", two)
			},
			func(svt *ScopedVersionedTable[value]) {
				afterIfVariable := svt.ReadValue("a")
				test.Equal(afterIfVariable, NewPhi(two, one))
			},
			func(svt *ScopedVersionedTable[value]) {
				afterBlockVariable := svt.ReadValue("a")
				test.Equal(afterBlockVariable, NewPhi(two, one))
			},
		)
	})

	t.Run("test 2", func(t *testing.T) {
		/*
			{
				a = 1
				if c {
					a = 2
				}
				a // phi[1, 2]
			}
			a // nil
		*/
		test := assert.New(t)
		check(
			func(svt *ScopedVersionedTable[value]) {},
			func(svt *ScopedVersionedTable[value]) {
				svt.WriteVariable("a", one)
			},
			func(svt *ScopedVersionedTable[value]) {
				svt.WriteVariable("a", two)
			},
			func(svt *ScopedVersionedTable[value]) {
				afterIfVariable := svt.ReadValue("a")
				test.Equal(afterIfVariable, NewPhi(two, one))
			},
			func(svt *ScopedVersionedTable[value]) {
				afterBlockVariable := svt.ReadValue("a")
				test.Nil(afterBlockVariable)
			},
		)
	})
}
func TestLoopScope_Basic(t *testing.T) {
	/*
		beforeLoop()
		for loopFirst(); loopCondition(); loopThird() {
			loopBody()
		}
		afterLoop()
	*/
	build := func(
		beforeLoop, loopFirst, loopCondition BuilderFunc,
		body BuilderReturnScopeFunc,
		loopThird, afterLoop BuilderFunc,
	) {
		global := NewRootVersionedTable[value](NewVersioned[value])
		beforeLoop(global)

		builder := NewLoopStmt(global, NewPhiValue)
		builder.SetFirst(func(sub *ScopedVersionedTable[value]) {
			loopFirst(sub)
		})
		builder.SetCondition(func(sub *ScopedVersionedTable[value]) {
			loopCondition(sub)
		})
		builder.SetThird(func(sub *ScopedVersionedTable[value]) {
			loopThird(sub)
		})
		builder.SetBody(func(sub *ScopedVersionedTable[value]) *ScopedVersionedTable[value] {
			return BuildSyntaxBlock(sub, func(sub *ScopedVersionedTable[value]) *ScopedVersionedTable[value] {
				return body(sub)
			})
		})
		end := builder.Build(SpinHandler, GeneratePhi)
		afterLoop(end)
	}

	t.Run("test basic", func(t *testing.T) {
		/*
			{
				i = 1
				for i < 10 { // phi(1, 2)
					i = 2
				}
				i // phi
			}
		*/
		var conditionVariable, endVariable value
		build(
			func(svt *ScopedVersionedTable[value]) {
				svt.WriteVariable("i", one)
			},
			func(svt *ScopedVersionedTable[value]) {},
			func(svt *ScopedVersionedTable[value]) {
				conditionVariable = svt.ReadValue("i")
			},
			func(svt *ScopedVersionedTable[value]) *ScopedVersionedTable[value] {
				svt.WriteVariable("i", two)
				return svt
			},
			func(svt *ScopedVersionedTable[value]) {},
			func(svt *ScopedVersionedTable[value]) {
				endVariable = svt.ReadValue("i")
			},
		)
		test := assert.New(t)
		test.Equal("phi[const(1) const(2)]", conditionVariable.String())
		test.Equal("phi[const(1) const(2)]", endVariable.String())
	})

	t.Run("test spin", func(t *testing.T) {
		/*
			i = 0
			for i < 10 { // t2
				i = i + 1
				// t2 = phi(t1, 0)
				// t1 = t2 + 1
			}
			i // t2 phi(0, $+1)
		*/

		var conditionVariable, Binary, endVariable value
		build(
			func(svt *ScopedVersionedTable[value]) {
				svt.WriteVariable("i", zero)
			},
			func(svt *ScopedVersionedTable[value]) {},
			func(svt *ScopedVersionedTable[value]) {
				conditionVariable = svt.ReadValue("i")
			},
			func(svt *ScopedVersionedTable[value]) *ScopedVersionedTable[value] {
				bodyVariable := svt.ReadValue("i")
				Binary = NewBinary(bodyVariable, one)
				svt.WriteVariable("i", Binary)
				return svt
			},
			func(svt *ScopedVersionedTable[value]) {},
			func(svt *ScopedVersionedTable[value]) {
				endVariable = svt.ReadValue("i")
			},
		)

		test := assert.New(t)
		test.Equal(*NewPhi(zero, Binary), *conditionVariable.(*phi))
		test.Equal(*NewPhi(zero, Binary), *endVariable.(*phi))
		test.Equal(*NewBinary(conditionVariable, one), *Binary.(*binary))
	})

	t.Run("test spin with first", func(t *testing.T) {
		/*
			for i=1; i<10; i++ {
				i // phi[1, i+1]
			}
			i // undefine
		*/

		var conditionVariable, bodyVariable, thirdVariable, thirdVariableBinary, endVariable value
		build(
			func(svt *ScopedVersionedTable[value]) {},
			func(svt *ScopedVersionedTable[value]) {
				svt.WriteVariable("i", one)
			},
			func(svt *ScopedVersionedTable[value]) {
				conditionVariable = svt.ReadValue("i")
			},
			func(svt *ScopedVersionedTable[value]) *ScopedVersionedTable[value] {
				bodyVariable = svt.ReadValue("i")
				return svt
			},
			func(svt *ScopedVersionedTable[value]) {
				thirdVariable = svt.ReadValue("i")
				thirdVariableBinary = NewBinary(thirdVariable, one)
				svt.WriteVariable("i", thirdVariableBinary)
			},
			func(svt *ScopedVersionedTable[value]) {
				endVariable = svt.ReadValue("i")
			},
		)
		test := assert.New(t)
		test.Equal(*NewPhi(one, thirdVariableBinary), *conditionVariable.(*phi))
		test.Equal(conditionVariable, bodyVariable)
		test.Equal(conditionVariable, thirdVariable)
		test.Equal(*NewBinary(conditionVariable, one), *thirdVariableBinary.(*binary))
		test.Nil(endVariable)
	})

	t.Run("test undefine variable", func(t *testing.T) {
		/*
			for {
				println // nil
			}
			println // nil

		*/
		test := assert.New(t)
		build(
			func(svt *ScopedVersionedTable[value]) {},
			func(svt *ScopedVersionedTable[value]) {},
			func(svt *ScopedVersionedTable[value]) {},
			func(svt *ScopedVersionedTable[value]) *ScopedVersionedTable[value] {
				bodyVariable := svt.ReadValue("println")
				test.Nil(bodyVariable)
				return svt
			},
			func(svt *ScopedVersionedTable[value]) {},
			func(svt *ScopedVersionedTable[value]) {
				endVariable := svt.ReadValue("println")
				test.Nil(endVariable)
			},
		)

	})
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

	global := NewRootVersionedTable[value](NewVersioned[value])
	zero := NewConsts("0")
	one := NewConsts("1")
	two := NewConsts("2")
	global.WriteVariable("i", zero)

	var conditionVariable, bodyVariablePhi, trueVariableBinary, falseVariableBinary, endVariablePhi value
	builder := NewLoopStmt(global, NewPhiValue)
	builder.SetCondition(func(sub *ScopedVersionedTable[value]) {
		conditionVariable = sub.ReadValue("i")
	})
	builder.SetBody(func(body *ScopedVersionedTable[value]) *ScopedVersionedTable[value] {
		return BuildSyntaxBlock(body, func(body *ScopedVersionedTable[value]) *ScopedVersionedTable[value] {
			build := NewIfStmt(body)
			build.BuildItem(
				func(*ScopedVersionedTable[value]) {},
				func(body *ScopedVersionedTable[value]) *ScopedVersionedTable[value] {
					return BuildSyntaxBlock(body, func(body *ScopedVersionedTable[value]) *ScopedVersionedTable[value] {
						trueVariablePhi := body.ReadValue("i")
						body.WriteVariable("i", NewBinary(trueVariablePhi, one))
						trueVariableBinary = body.ReadValue("i")
						return body
					})
				},
			)
			build.BuildElse(func(body *ScopedVersionedTable[value]) *ScopedVersionedTable[value] {
				return BuildSyntaxBlock(body, func(body *ScopedVersionedTable[value]) *ScopedVersionedTable[value] {
					falseVariablePhi := body.ReadValue("i")
					body.WriteVariable("i", NewBinary(falseVariablePhi, two))
					falseVariableBinary = body.ReadValue("i")
					return body
				})
			})
			end := build.BuildFinish(GeneratePhi)
			bodyVariablePhi = end.ReadValue("i")
			return end
		})
	})
	end := builder.Build(SpinHandler, GeneratePhi)

	endVariablePhi = end.ReadValue("i")

	test.Equal(*NewPhi(trueVariableBinary, falseVariableBinary), *bodyVariablePhi.(*phi))
	test.Equal(*NewPhi(zero, bodyVariablePhi), *endVariablePhi.(*phi))
	test.Equal(conditionVariable, endVariablePhi)
}

func TestIfLoopScope_Break(t *testing.T) {
	/*
		i = 0
		for i=1; i<10; i++ {
			if i == 2 {
				i = 2
				break
			}
		}
		i // phi[2, phi[1+$]]
	*/

	test := assert.New(t)
	global := NewRootVersionedTable[value](NewVersioned[value])
	global.WriteVariable("i", zero)

	var ConditionVariable, ThirdVariable1, TrueVariable, BinaryVariable value

	LoopBuilder := NewLoopStmt(global, NewPhiValue)
	LoopBuilder.SetFirst(func(sub *ScopedVersionedTable[value]) {
		sub.WriteVariable("i", one)
	})
	LoopBuilder.SetCondition(func(sub *ScopedVersionedTable[value]) {
		ConditionVariable = sub.ReadValue("i")
	})
	LoopBuilder.SetThird(func(sub *ScopedVersionedTable[value]) {
		ThirdVariable1 = sub.ReadValue("i")
		BinaryVariable = NewBinary(ThirdVariable1, one)
		sub.WriteVariable("i", BinaryVariable)
	})
	LoopBuilder.SetBody(func(sub *ScopedVersionedTable[value]) *ScopedVersionedTable[value] {
		return BuildSyntaxBlock(sub, func(sub *ScopedVersionedTable[value]) *ScopedVersionedTable[value] {
			Build := NewIfStmt(sub)
			Build.BuildItem(
				func(sub *ScopedVersionedTable[value]) {
					TrueVariable = sub.ReadValue("i")
				},
				func(sub *ScopedVersionedTable[value]) *ScopedVersionedTable[value] {
					sub.WriteVariable("i", two)
					LoopBuilder.Break(sub)
					return nil
				})
			end := Build.BuildFinish(GeneratePhi)
			return end
		})
	})

	end := LoopBuilder.Build(SpinHandler, GeneratePhi)
	endVariable := end.ReadValue("i")

	test.Equal(*NewPhi(one, BinaryVariable), *ConditionVariable.(*phi))
	test.Equal(ThirdVariable1, ConditionVariable)
	test.Equal(ThirdVariable1, TrueVariable)
	test.Equal(*NewPhi(two, ThirdVariable1), *endVariable.(*phi))
}
func TestIfLoopScope_Continue(t *testing.T) {
	/*
		i = 0
		for i=1; i<10; i++ { // i phi[1, $+1, 2]
			if i == 2 {
				i = 2
				continue
			}
		}
		i
	*/

	test := assert.New(t)
	global := NewRootVersionedTable[value](NewVersioned[value])
	global.WriteVariable("i", zero)

	var ConditionVariable, ThirdVariable1, TrueVariable, BinaryVariable value

	LoopBuilder := NewLoopStmt(global, NewPhiValue)
	LoopBuilder.SetFirst(func(sub *ScopedVersionedTable[value]) {
		sub.WriteVariable("i", one)
	})
	LoopBuilder.SetCondition(func(sub *ScopedVersionedTable[value]) {
		ConditionVariable = sub.ReadValue("i")
	})
	LoopBuilder.SetThird(func(sub *ScopedVersionedTable[value]) {
		ThirdVariable1 = sub.ReadValue("i")
		BinaryVariable = NewBinary(ThirdVariable1, one)
		sub.WriteVariable("i", BinaryVariable)
	})
	LoopBuilder.SetBody(func(sub *ScopedVersionedTable[value]) *ScopedVersionedTable[value] {
		return BuildSyntaxBlock(sub, func(sub *ScopedVersionedTable[value]) *ScopedVersionedTable[value] {
			Build := NewIfStmt(sub)
			Build.BuildItem(
				func(sub *ScopedVersionedTable[value]) {
					TrueVariable = sub.ReadValue("i")
				},
				func(sub *ScopedVersionedTable[value]) *ScopedVersionedTable[value] {
					sub.WriteVariable("i", two)
					LoopBuilder.Continue(sub)
					return nil
				})
			end := Build.BuildFinish(GeneratePhi)
			return end
		})
	})

	end := LoopBuilder.Build(SpinHandler, GeneratePhi)
	endVariable := end.ReadValue("i")

	test.Equal(*NewPhi(two, ConditionVariable), *ThirdVariable1.(*phi))
	test.Equal(*NewPhi(one, BinaryVariable), *ConditionVariable.(*phi))
	test.Equal(ConditionVariable, TrueVariable)
	test.Equal(ConditionVariable, endVariable)
}
