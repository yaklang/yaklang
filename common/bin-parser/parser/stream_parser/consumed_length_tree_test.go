package stream_parser

import (
	"fmt"
	"math"
	"math/rand"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
)

type consumedTreeOutcome struct {
	length uint64
	panic  string
}

func consumedTreeCall(fn func(*base.Node) uint64, node *base.Node) (out consumedTreeOutcome) {
	defer func() {
		if recovered := recover(); recovered != nil {
			out.panic = fmt.Sprintf("%T: %v", recovered, recovered)
		}
	}()
	out.length = fn(node)
	return out
}

func consumedTreeNode(children ...*base.Node) *base.Node {
	return &base.Node{Name: "test", Cfg: base.NewEmptyConfig(), Children: children}
}

func consumedTreeSpan(start, end uint64, children ...*base.Node) *base.Node {
	node := consumedTreeNode(children...)
	node.Cfg.SetItem(CfgNodeResult, [2]uint64{start, end})
	return node
}

func consumedTreeRequireEqual(t *testing.T, node *base.Node) consumedTreeOutcome {
	t.Helper()
	want := consumedTreeCall(calcNodeConsumedLengthLegacy, node)
	for _, implementation := range []struct {
		name string
		fn   func(*base.Node) uint64
	}{
		{"direct", calcNodeConsumedLength},
		{"public", CalcNodeConsumedLength},
	} {
		if got := consumedTreeCall(implementation.fn, node); got != want {
			t.Fatalf("%s = %+v, legacy = %+v", implementation.name, got, want)
		}
	}
	return want
}

func TestConsumedLengthTreeSemantics(t *testing.T) {
	invalidResult := func() *base.Node {
		node := consumedTreeNode()
		node.Cfg.SetItem(CfgNodeResult, "not a result span")
		return node
	}
	tests := []struct {
		name  string
		build func() *base.Node
		want  uint64
	}{
		{"empty", func() *base.Node { return consumedTreeNode() }, 0},
		{"unaligned_span", func() *base.Node { return consumedTreeSpan(3, 20) }, 17},
		{"zero_span_shadows_descendants", func() *base.Node {
			return consumedTreeSpan(11, 11, invalidResult())
		}, 0},
		{"span_shadows_descendants", func() *base.Node {
			return consumedTreeSpan(7, 19, consumedTreeSpan(0, 100), invalidResult())
		}, 12},
		{"unparsed_consumed_ignored", func() *base.Node {
			node := consumedTreeNode(consumedTreeSpan(7, 12), consumedTreeSpan(20, 31))
			node.Cfg.SetItem(CfgConsumedBits, uint64(1000))
			return node
		}, 16},
		{"unparsed_nil_consumed_ignored", func() *base.Node {
			node := consumedTreeNode(consumedTreeSpan(0, 9))
			node.Cfg.SetItem(CfgConsumedBits, nil)
			return node
		}, 9},
		{"unparsed_invalid_consumed_ignored", func() *base.Node {
			node := consumedTreeNode(consumedTreeSpan(0, 13))
			node.Cfg.SetItem(CfgConsumedBits, "13")
			return node
		}, 13},
		{"consumed_override_shadows_invalid_result", func() *base.Node {
			node := invalidResult()
			node.Cfg.SetItem(CfgConsumedBits, uint64(27))
			node.Children = []*base.Node{invalidResult()}
			return node
		}, 27},
		{"consumed_override_shadows_nil_result", func() *base.Node {
			node := consumedTreeNode(invalidResult())
			node.Cfg.SetItem(CfgNodeResult, nil)
			node.Cfg.SetItem(CfgConsumedBits, uint64(23))
			return node
		}, 23},
		{"repeated_child_pointer_counted_per_occurrence", func() *base.Node {
			child := consumedTreeSpan(8, 15)
			return consumedTreeNode(child, consumedTreeNode(child), child)
		}, 21},
		{"span_subtraction_wrap", func() *base.Node { return consumedTreeSpan(5, 3) }, math.MaxUint64 - 1},
		{"sibling_addition_wrap", func() *base.Node {
			return consumedTreeNode(consumedTreeSpan(0, math.MaxUint64), consumedTreeSpan(0, 9))
		}, 8},
		{"nested_addition_wrap", func() *base.Node {
			return consumedTreeNode(consumedTreeSpan(0, 6), consumedTreeNode(consumedTreeSpan(0, math.MaxUint64), consumedTreeSpan(0, 9)))
		}, 14},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := consumedTreeRequireEqual(t, test.build()); got.panic != "" || got.length != test.want {
				t.Fatalf("got %+v, want length %d without panic", got, test.want)
			}
		})
	}
}

func TestConsumedLengthTreeConsumedTypes(t *testing.T) {
	type namedUint64 uint64
	type namedInt int
	var nilPointer *uint64
	tests := []struct {
		name  string
		value any
		want  uint64
	}{
		{"uint64", uint64(41), 41},
		{"uint32", uint32(41), 41},
		{"uint16", uint16(41), 41},
		{"uint8", uint8(41), 41},
		{"int64", int64(41), 41},
		{"int32", int32(41), 41},
		{"int16", int16(41), 41},
		{"int8", int8(41), 41},
		{"int", int(41), 41},
		{"float64_fraction", float64(41.75), 41},
		{"float32_fraction", float32(41.75), 41},
		{"uint64_max", uint64(math.MaxUint64), math.MaxUint64},
		{"negative_int64", int64(-1), math.MaxUint64},
		{"negative_int32", int32(-2), math.MaxUint64 - 1},
		{"negative_int16", int16(-3), math.MaxUint64 - 2},
		{"negative_int8", int8(-4), math.MaxUint64 - 3},
		{"negative_int", int(-5), math.MaxUint64 - 4},
		// Preserve the existing converter's rejected types, even numeric ones.
		{"unsupported_uint", uint(41), 0},
		{"unsupported_uintptr", uintptr(41), 0},
		{"unsupported_complex64", complex64(41), 0},
		{"unsupported_complex128", complex128(41), 0},
		{"unsupported_named_uint64", namedUint64(41), 0},
		{"unsupported_named_int", namedInt(41), 0},
		{"nil", nil, 0},
		{"typed_nil", nilPointer, 0},
		{"numeric_string", "41", 0},
		{"bool", true, 0},
		{"slice", []byte{41}, 0},
		{"map", map[string]int{"length": 41}, 0},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			node := consumedTreeSpan(8, 24, consumedTreeSpan(0, 100))
			node.Cfg.SetItem(CfgConsumedBits, test.value)
			if got := consumedTreeRequireEqual(t, node); got.panic != "" || got.length != test.want {
				t.Fatalf("got %+v, want length %d without panic", got, test.want)
			}
		})
	}
	// Out-of-range float conversion is platform-dependent; retain equality to
	// the original conversion without asserting an architecture-specific value.
	for _, value := range []any{math.NaN(), math.Inf(1), math.Inf(-1), -1.5, math.MaxFloat64, float32(math.MaxFloat32)} {
		node := consumedTreeSpan(0, 10)
		node.Cfg.SetItem(CfgConsumedBits, value)
		if got := consumedTreeRequireEqual(t, node); got.panic != "" {
			t.Fatalf("float %v unexpectedly panicked: %s", value, got.panic)
		}
	}
}

func TestConsumedLengthTreeResultPanics(t *testing.T) {
	type namedSpan [2]uint64
	var nilSpan *[2]uint64
	for _, test := range []struct {
		name   string
		result any
	}{
		{"nil", nil},
		{"typed_nil", nilSpan},
		{"string", "[0, 8]"},
		{"slice", []uint64{0, 8}},
		{"wrong_array_length", [3]uint64{0, 8}},
		{"wrong_integer_type", [2]int{0, 8}},
		{"named_array_type", namedSpan{0, 8}},
	} {
		t.Run(test.name, func(t *testing.T) {
			node := consumedTreeNode(consumedTreeSpan(0, 11))
			node.Cfg.SetItem(CfgNodeResult, test.result)
			if got := consumedTreeRequireEqual(t, node); !strings.Contains(got.panic, "interface conversion") {
				t.Fatalf("invalid result must retain the legacy type assertion panic: %+v", got)
			}
			// Presence of an invalid consumed override still suppresses the result
			// assertion, rather than falling through to the result or descendants.
			node.Cfg.SetItem(CfgConsumedBits, nil)
			if got := consumedTreeRequireEqual(t, node); got.panic != "" || got.length != 0 {
				t.Fatalf("nil consumed override: %+v", got)
			}
		})
	}
}

func TestConsumedLengthTreePublicMutations(t *testing.T) {
	child := consumedTreeSpan(2, 10)
	other := consumedTreeSpan(20, 23)
	root := consumedTreeNode(child, other)
	check := func(want uint64) {
		t.Helper()
		if got := consumedTreeRequireEqual(t, root); got.panic != "" || got.length != want {
			t.Fatalf("got %+v, want length %d without panic", got, want)
		}
	}
	check(11)
	child.Cfg.SetItem(CfgNodeResult, [2]uint64{10, 27})
	check(20)
	child.Cfg.SetItem(CfgConsumedBits, uint64(31))
	check(34)
	child.Cfg.SetItem(CfgConsumedBits, nil)
	check(3)
	child.Cfg.DeleteItem(CfgConsumedBits)
	check(20)
	// Public BaseKV writes bypass Config.SetItem; they must remain visible.
	child.Cfg.BaseKV.SetItem(CfgConsumedBits, uint64(6))
	check(9)
	child.Cfg.BaseKV.DeleteItem(CfgConsumedBits)
	check(20)
	child.Cfg.BaseKV.SetItem(CfgNodeResult, [2]uint64{100, 104})
	check(7)
	child.Cfg.BaseKV.DeleteItem(CfgNodeResult)
	check(3)
	child.Children = []*base.Node{consumedTreeSpan(0, 9)}
	check(12)
	child.Cfg.SetItem(CfgConsumedBits, uint64(100)) // Ignored without a result.
	check(12)
	child.Cfg = base.NewEmptyConfig()
	child.Cfg.SetItem(CfgNodeResult, [2]uint64{8, 23})
	check(18)
	child.Cfg.DeleteItem(CfgNodeResult)
	check(12)
	// Replacing an entry without changing slice length defeats a length-only
	// prefix cache; replacing the entire same-sized slice must also stay live.
	root.Children[0] = consumedTreeSpan(0, 29)
	check(32)
	root.Children = []*base.Node{consumedTreeSpan(0, 37), other}
	check(40)
	root.Children = []*base.Node{other, other}
	check(6)
	other.Cfg.SetItem(CfgNodeResult, [2]uint64{0, 5})
	check(10)
	root.Cfg.SetItem(CfgNodeResult, [2]uint64{0, 19})
	check(19)
	root.Cfg.DeleteItem(CfgNodeResult)
	check(10)
}

func consumedTreeRandomValue(rng *rand.Rand) any {
	switch rng.Intn(12) {
	case 0:
		return nil
	case 1:
		return "17"
	case 2:
		return uint64(rng.Uint64())
	case 3:
		return int64(rng.Uint64())
	case 4:
		return int(rng.Intn(1024))
	case 5:
		return uint32(rng.Uint32())
	case 6:
		return int8(rng.Intn(256))
	case 7:
		return float64(rng.Intn(1024)) + 0.5
	case 8:
		return float32(rng.Intn(1024)) + 0.25
	case 9:
		return uint(rng.Intn(1024))
	case 10:
		return []uint64{1, 2}
	default:
		return false
	}
}

func TestConsumedLengthTreeDeterministicDifferential(t *testing.T) {
	const seed = int64(0x636f6e73756d6564)
	rng := rand.New(rand.NewSource(seed))
	for iteration := 0; iteration < 1024; iteration++ {
		var nodes []*base.Node
		var build func(int) *base.Node
		build = func(depth int) *base.Node {
			node := consumedTreeNode()
			nodes = append(nodes, node)
			switch rng.Intn(5) {
			case 0:
				node.Cfg.BaseKV.SetItem(CfgNodeResult, [2]uint64{rng.Uint64(), rng.Uint64()})
			case 1:
				node.Cfg.BaseKV.SetItem(CfgNodeResult, consumedTreeRandomValue(rng))
			}
			if rng.Intn(3) == 0 {
				node.Cfg.BaseKV.SetItem(CfgConsumedBits, consumedTreeRandomValue(rng))
			}
			if depth > 0 && len(nodes) < 128 {
				for count := rng.Intn(4); count > 0; count-- {
					node.Children = append(node.Children, build(depth-1))
				}
				if len(node.Children) > 0 && rng.Intn(5) == 0 {
					node.Children = append(node.Children, node.Children[0])
				}
			}
			return node
		}
		root := build(6)
		// Keep the root open so generated descendants are actually traversed.
		root.Cfg.DeleteItem(CfgNodeResult)
		for mutation := 0; mutation < 8; mutation++ {
			legacy := consumedTreeCall(calcNodeConsumedLengthLegacy, root)
			fast := consumedTreeCall(calcNodeConsumedLength, root)
			if fast != legacy {
				t.Fatalf("seed=%d iteration=%d mutation=%d nodes=%d: direct=%+v legacy=%+v", seed, iteration, mutation, len(nodes), fast, legacy)
			}
			target := nodes[rng.Intn(len(nodes))]
			switch mutation {
			case 0:
				target.Cfg.SetItem(CfgNodeResult, [2]uint64{rng.Uint64(), rng.Uint64()})
			case 1:
				target.Cfg.BaseKV.SetItem(CfgConsumedBits, consumedTreeRandomValue(rng))
			case 2:
				target.Cfg.DeleteItem(CfgNodeResult)
			case 3:
				target.Cfg.BaseKV.DeleteItem(CfgConsumedBits)
			case 4:
				if len(target.Children) > 0 {
					target.Children[0] = consumedTreeSpan(rng.Uint64(), rng.Uint64())
				}
			case 5:
				target.Cfg = base.NewEmptyConfig()
			case 6:
				target.Children = []*base.Node{consumedTreeSpan(0, 3), consumedTreeSpan(0, 11)}
			}
		}
	}
}

func consumedTreeFlat(width int) *base.Node {
	root := consumedTreeNode()
	for i := 0; i < width; i++ {
		child := consumedTreeSpan(uint64(i*8), uint64(i*8+5))
		if i%3 == 0 {
			child.Cfg.SetItem(CfgConsumedBits, uint64(8))
		}
		root.Children = append(root.Children, child)
	}
	return root
}

func consumedTreeDeep(depth int) *base.Node {
	root := consumedTreeSpan(1, 9)
	for i := 0; i < depth; i++ {
		root = consumedTreeNode(consumedTreeSpan(0, 3), root)
	}
	return root
}

func TestConsumedLengthTreeDepthAndWidth(t *testing.T) {
	for _, width := range []int{16, 256, 4096} {
		t.Run(fmt.Sprintf("flat_%d", width), func(t *testing.T) {
			want := uint64(width*5 + ((width+2)/3)*3)
			if got := consumedTreeRequireEqual(t, consumedTreeFlat(width)); got.panic != "" || got.length != want {
				t.Fatalf("got %+v, want %d", got, want)
			}
		})
	}
	for _, depth := range []int{16, 128, 512} {
		t.Run(fmt.Sprintf("deep_%d", depth), func(t *testing.T) {
			want := uint64(8 + depth*3)
			if got := consumedTreeRequireEqual(t, consumedTreeDeep(depth)); got.panic != "" || got.length != want {
				t.Fatalf("got %+v, want %d", got, want)
			}
		})
	}
}

var consumedTreeBenchmarkSink uint64

// Run one size/path independently, for example:
//
//	go test ./common/bin-parser/parser/stream_parser -run '^$' \
//	  -bench '^BenchmarkConsumedLengthTree/Flat_256/(Legacy|Direct)$' -benchmem -cpu=1
func BenchmarkConsumedLengthTree(b *testing.B) {
	for _, tree := range []struct {
		name  string
		build func() *base.Node
	}{
		{"Flat_16", func() *base.Node { return consumedTreeFlat(16) }},
		{"Flat_256", func() *base.Node { return consumedTreeFlat(256) }},
		{"Flat_4096", func() *base.Node { return consumedTreeFlat(4096) }},
		{"Deep_16", func() *base.Node { return consumedTreeDeep(16) }},
		{"Deep_128", func() *base.Node { return consumedTreeDeep(128) }},
		{"Deep_512", func() *base.Node { return consumedTreeDeep(512) }},
	} {
		b.Run(tree.name, func(b *testing.B) {
			node := tree.build()
			want := calcNodeConsumedLengthLegacy(node)
			for _, implementation := range []struct {
				name string
				fn   func(*base.Node) uint64
			}{
				{"Legacy", calcNodeConsumedLengthLegacy},
				{"Direct", calcNodeConsumedLength},
			} {
				b.Run(implementation.name, func(b *testing.B) {
					if got := implementation.fn(node); got != want {
						b.Fatalf("got %d, want %d", got, want)
					}
					b.ReportAllocs()
					b.ResetTimer()
					var result uint64
					for i := 0; i < b.N; i++ {
						result = implementation.fn(node)
					}
					consumedTreeBenchmarkSink = result
				})
			}
		})
	}
}
