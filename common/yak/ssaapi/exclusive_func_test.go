package ssaapi

import "testing"

func TestFunctionTrace(t *testing.T) {
	prog, err := Parse(`c =(i=>i)(1);a = {};a.b=c;e=a.b;dump(e)`)
	if err != nil {
		t.Fatal(err)
	}
	prog.Show().Ref("e").ForEach(func(value *Value) {
		value.GetTopDefs().ForEach(func(value *Value) {
			t.Log(value.String())
		})
	})
}

func TestFunctionTrace_FormalParametersCheck(t *testing.T) {
	prog, err := Parse(`
a = 1
b = (c, d) => {
	a = c + d
	return d, c
}
f = b(2,3)
`)
	if err != nil {
		t.Fatal(err)
	}
	prog.Show()

	check2 := false
	check3 := false
	prog.Ref("f").Show().ForEach(func(value *Value) {
		value.GetTopDefs().ForEach(func(value *Value) {
			if value.IsConstInst() {
				if value.GetConstValue() == 2 {
					check2 = true
				}
				if value.GetConstValue() == 3 {
					check3 = true
				}
			}
		})
	})

	if !check2 {
		t.Fatal("the literal 2 trace failed")
	}
	if !check3 {
		t.Fatal("the literal 3 trace failed")
	}
}

func TestFunctionTrace_FormalParametersCheck_2(t *testing.T) {
	prog, err := Parse(`
a = 1
b = (c, d, e) => {
	a = c + d
	return d, c
}
f = b(2,3,4)
`)
	if err != nil {
		t.Fatal(err)
	}
	prog.Show()

	check2 := false
	check3 := false
	noCheck4 := true
	prog.Ref("f").Show().ForEach(func(value *Value) {
		value.GetTopDefs().ForEach(func(value *Value) {
			d := value.Dot()
			_ = d
			value.ShowDot()
			if value.IsConstInst() {
				if value.GetConstValue() == 2 {
					check2 = true
				}
				if value.GetConstValue() == 3 {
					check3 = true
				}
				if value.GetConstValue() == 4 {
					noCheck4 = false
				}
			}
		})
	})

	if !noCheck4 {
		t.Fatal("literal 4 should not be traced")
	}

	if !check2 {
		t.Fatal("the literal 2 trace failed")
	}
	if !check3 {
		t.Fatal("the literal 3 trace failed")
	}
}

func TestDepthLimit(t *testing.T) {
	prog, err := Parse(`var a;
b = a+1
c = b + e;
d = c + f;
g = d
`)
	if err != nil {
		t.Fatal(err)
	}

	depth2check := false
	depthAllcheck := false
	prog.Ref("g").ForEach(func(value *Value) {
		var count int
		value.GetTopDefs(WithMaxDepth(2)).ForEach(func(value *Value) {
			count++
		})
		if count == 3 {
			depth2check = true
		}

		count = 0
		value.GetTopDefs(WithMaxDepth(-1)).ForEach(func(value *Value) {
			count++
		})
		if count == 4 {
			depthAllcheck = true
		}
	})

	if !depth2check {
		t.Fatal("depth2check failed")
	}

	if !depthAllcheck {
		t.Fatal("depthAllcheck failed")
	}
}

func TestDominatorTree(t *testing.T) {
	prog, err := Parse(`var a;
b = a+1
c = b + e;
d = c + f;
g = d
`)
	if err != nil {
		t.Fatal(err)
	}

	depth2check := false
	depthAllcheck := false
	prog.Ref("g").ForEach(func(value *Value) {
		var count int
		value.GetTopDefs(WithMaxDepth(2)).ForEach(func(value *Value) {
			count++
		})
		if count == 3 {
			depth2check = true
		}

		count = 0
		value.GetTopDefs(WithMaxDepth(-1)).ForEach(func(value *Value) {
			count++
		})
		if count == 4 {
			depthAllcheck = true
		}
	})

	if !depth2check {
		t.Fatal("depth2check failed")
	}

	if !depthAllcheck {
		t.Fatal("depthAllcheck failed")
	}
}
