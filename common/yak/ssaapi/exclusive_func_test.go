package ssaapi

import "testing"

func TestFunctionTrace(t *testing.T) {
	prog, err := Parse(`
a = 1
b = (c, d) => {
	a = c + d
	return d, c
}
e, f = b(2,3)
g = e // 3
h = f // 2
i = a // 2 + 3
`)
	if err != nil {
		t.Fatal(err)
	}
	prog.Ref("h").ForEach(func(value *Value) {
		value.GetTopDefs().ForEach(func(value *Value) {
			value.ShowBacktrack()
		})
	})
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
