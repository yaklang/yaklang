package stream_parser_test

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	_ "github.com/yaklang/yaklang/common/bin-parser/parser"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
	yaml "github.com/yaklang/yaklang/common/utils/orderedyaml"
)

// These tests deliberately retain non-monotone operator behavior. The public
// tree's consumed length is not necessarily the writer's current bit offset.
// The switch is set explicitly on both sides so the same build can compare
// the original tree walk with the optimized live traversal, without caching
// mutable configuration, topology, or completed consumption across calls.
type consumedLengthValue struct {
	Name     string
	List     bool
	Struct   bool
	Value    any
	Children []consumedLengthValue
}

type consumedLengthNode struct {
	Name        string
	Origin      any
	Config      map[string]any
	Consumed    uint64
	Result      *consumedLengthValue
	ResultError string
	SharesCtx   bool
	Children    []consumedLengthNode
}

func consumedLengthDiagnostic(err error) string {
	if err == nil {
		return ""
	}
	message := err.Error()
	if at := strings.LastIndex(message, "YakVM Panic: "); at >= 0 {
		message = message[at+len("YakVM Panic: "):]
	}
	return strings.SplitN(message, "\n", 2)[0]
}

func consumedLengthSnapshot(t *testing.T, node *base.Node) consumedLengthNode {
	t.Helper()
	snapshot := consumedLengthNode{Name: node.Name, Origin: node.Origin, Config: map[string]any{}, Consumed: stream_parser.CalcNodeConsumedLength(node)}
	for _, key := range []string{
		base.CfgType, base.CfgLength, base.CfgIsList, base.CfgIsTerminal, base.CfgNodeResult,
		base.CfgEndian, base.CfgImport, base.CfgOperator, base.CfgDelimiter, base.CfgDel,
		stream_parser.CfgConsumedBits, stream_parser.CfgLengthForStartField,
		stream_parser.CfgLengthForField, stream_parser.CfgLengthFromField,
		stream_parser.CfgLengthCacheMap, stream_parser.CfgElementIndex,
		stream_parser.CfgDelimiterOptional, "additionInfo",
	} {
		if node.Cfg.Has(key) {
			snapshot.Config[key] = node.Cfg.GetItem(key)
		}
	}
	if parent, ok := node.Cfg.GetItem(base.CfgParent).(*base.Node); ok {
		snapshot.SharesCtx = node.Ctx == parent.Ctx
	}
	var valueSnapshot func(*base.NodeValue) consumedLengthValue
	valueSnapshot = func(value *base.NodeValue) consumedLengthValue {
		v := consumedLengthValue{Name: value.Name, List: value.IsList(), Struct: value.IsStruct()}
		if value.IsValue() {
			v.Value = value.Value
		} else {
			for _, child := range value.Children() {
				v.Children = append(v.Children, valueSnapshot(child))
			}
		}
		return v
	}
	value, err := node.Result()
	if err != nil {
		snapshot.ResultError = consumedLengthDiagnostic(err)
	} else {
		require.Same(t, node, value.Origin)
		v := valueSnapshot(value)
		snapshot.Result = &v
	}
	for _, child := range node.Children {
		require.Same(t, node, child.Cfg.GetItem(base.CfgParent), child.Name)
		snapshot.Children = append(snapshot.Children, consumedLengthSnapshot(t, child))
	}
	return snapshot
}

type consumedLengthRun struct {
	root    *base.Node
	message *base.Node
	reader  *base.BitReader
	input   *bytes.Reader
	err     error
}

func consumedLengthParse(t *testing.T, source string, input []byte, legacy bool, setup func(*base.Node)) consumedLengthRun {
	t.Helper()
	var document yaml.MapSlice
	require.NoError(t, yaml.Unmarshal([]byte(source), &document))
	root, err := base.NewNodeTree(document)
	require.NoError(t, err)
	root.Cfg.SetItem(base.CfgLength, uint64(len(input)*8))
	root.Ctx.SetItem("parseConsumedLengthLegacy", legacy)
	root.Ctx.SetItem(base.CtxInputConfig, map[string]any{"parseConsumedLengthLegacy": legacy})
	if setup != nil {
		setup(root)
	}
	inputReader := bytes.NewReader(input)
	reader := base.NewBitReader(inputReader)
	err = root.Parse(reader)
	message := base.GetNodeByPath(root, "@Message")
	require.NotNil(t, message)
	return consumedLengthRun{root, message, reader, inputReader, err}
}

func consumedLengthRequireLeaf(t *testing.T, run consumedLengthRun, path string, span [2]uint64, value any, consumed uint64) {
	t.Helper()
	node := base.GetNodeByPath(run.root, path)
	require.NotNil(t, node, path)
	require.Equal(t, span, stream_parser.GetNodeResultPos(node), path)
	require.Equal(t, consumed, stream_parser.CalcNodeConsumedLength(node), path)
	result, err := node.Result()
	require.NoError(t, err, path)
	require.Same(t, node, result.Origin)
	require.Equal(t, value, result.Value, path)
}

func consumedLengthCompare(t *testing.T, fast, legacy consumedLengthRun) {
	t.Helper()
	require.Equal(t, consumedLengthDiagnostic(legacy.err), consumedLengthDiagnostic(fast.err), "actual failure cause")
	require.Equal(t, consumedLengthSnapshot(t, legacy.message), consumedLengthSnapshot(t, fast.message))
	require.Equal(t, legacy.root.Ctx.GetUint64("pointer"), fast.root.Ctx.GetUint64("pointer"))
	require.Equal(t, legacy.root.Ctx.GetItem("buffer").(*bytes.Buffer).Bytes(), fast.root.Ctx.GetItem("buffer").(*bytes.Buffer).Bytes())
	require.Equal(t, legacy.root.Ctx.GetItem("writer").(*base.BitWriter).Snapshot(), fast.root.Ctx.GetItem("writer").(*base.BitWriter).Snapshot())
	require.Equal(t, legacy.input.Len(), fast.input.Len())
}

func TestConsumedLengthDifferentialActiveAndNonMonotoneNodes(t *testing.T) {
	for _, tc := range []struct {
		name, source string
		input        []byte
		check        func(*testing.T, consumedLengthRun)
	}{
		{
			name: "active sub-bit body",
			source: `
Package:
  Message:
    operator: |
      body = this.GetSubNode("Body")
      body.SetMaxLength(2)
      if body.Length("bit") != 0 || body.GetMaxLength("bit") != 16 { panic("initial active body") }
      body.Process()
      if this.Length("bit") != 16 || this.GetSubNode("Tail").GetMaxLength() != 1 { panic("completed body boundary") }
      this.ProcessSubNode("Tail")
    Body:
      operator: |
        this.ProcessSubNode("A")
        if this.Length("bit") != 3 || this.GetMaxLength("bit") != 16 { panic("active partial body") }
        this.ProcessSubNode("B")
        if this.Length("bit") != 8 { panic("active complete octet") }
        this.ProcessSubNode("C")
      A: uint8,3bit
      B: uint8,5bit
      C: uint8
    Tail: raw
`,
			input: []byte{0xad, 0x73, 0xe1},
			check: func(t *testing.T, run consumedLengthRun) {
				consumedLengthRequireLeaf(t, run, "@Message.Body.A", [2]uint64{0, 3}, uint8(5), 3)
				consumedLengthRequireLeaf(t, run, "@Message.Body.B", [2]uint64{3, 8}, uint8(13), 5)
				consumedLengthRequireLeaf(t, run, "@Message.Body.C", [2]uint64{8, 16}, uint8(0x73), 8)
				consumedLengthRequireLeaf(t, run, "@Message.Tail", [2]uint64{16, 24}, []byte{0xe1}, 8)
			},
		},
		{
			name: "same terminal processed twice",
			source: `
Package:
  Message:
    operator: |
      this.ProcessSubNode("Byte")
      if this.Length() != 1 { panic("first value length") }
      this.ProcessSubNode("Byte")
      if this.Length() != 1 { panic("replaced result counted twice") }
      this.GetSubNode("Tail").SetMaxLength(1)
      this.ProcessSubNode("Tail")
      if this.Length() != 2 { panic("writer offset substituted for tree length") }
    Byte: uint8
    Tail: raw
`,
			input: []byte{0x11, 0x22, 0x33},
			check: func(t *testing.T, run consumedLengthRun) {
				consumedLengthRequireLeaf(t, run, "@Message.Byte", [2]uint64{8, 16}, uint8(0x22), 8)
				consumedLengthRequireLeaf(t, run, "@Message.Tail", [2]uint64{16, 24}, []byte{0x33}, 8)
				require.Equal(t, uint64(16), stream_parser.CalcNodeConsumedLength(run.message))
				require.Equal(t, uint64(24), run.root.Ctx.GetUint64("pointer"))
			},
		},
		{
			name: "operator order differs from schema order",
			source: `
Package:
  Message:
    operator: |
      this.ProcessSubNode("B")
      if this.Length() != 1 || this.GetSubNode("A").GetMaxLength() != 1 { panic("out-of-order active state") }
      this.ProcessSubNode("A")
      if this.Length() != 2 { panic("out-of-order consumed length") }
    A: uint8
    B: uint8
`,
			input: []byte{0x11, 0x22},
			check: func(t *testing.T, run consumedLengthRun) {
				consumedLengthRequireLeaf(t, run, "@Message.A", [2]uint64{8, 16}, uint8(0x22), 8)
				consumedLengthRequireLeaf(t, run, "@Message.B", [2]uint64{0, 8}, uint8(0x11), 8)
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fast := consumedLengthParse(t, tc.source, tc.input, false, nil)
			legacy := consumedLengthParse(t, tc.source, tc.input, true, nil)
			consumedLengthCompare(t, fast, legacy)
			for _, run := range []consumedLengthRun{fast, legacy} {
				require.NoError(t, run.err)
				require.Zero(t, run.input.Len())
				require.Equal(t, tc.input, run.root.Ctx.GetItem("buffer").(*bytes.Buffer).Bytes())
				tc.check(t, run)
			}
		})
	}
}

func TestConsumedLengthDifferentialDelimiterFraming(t *testing.T) {
	for prefix := 0; prefix < 8; prefix++ {
		t.Run(fmt.Sprintf("prefix-%d", prefix), func(t *testing.T) {
			prefixRule := ""
			if prefix > 0 {
				prefixRule = fmt.Sprintf("    Prefix: uint8,%dbit\n", prefix)
			}
			source := "Package:\n  Message:\n" + prefixRule + fmt.Sprintf("    Line:\n      type: string\n      del: \"\\n\"\n    Tail: uint8,%dbit\n", 8-prefix)
			// Independent bit construction: optional all-one prefix, A, LF,
			// then a suffix that closes the final octet.
			wire := make([]byte, 3)
			position := 0
			put := func(value, width int) {
				for bit := width - 1; bit >= 0; bit-- {
					wire[position/8] |= byte(value>>bit&1) << (7 - position%8)
					position++
				}
			}
			put((1<<prefix)-1, prefix)
			put('A', 8)
			put('\n', 8)
			tail := 0x55 & ((1 << (8 - prefix)) - 1)
			put(tail, 8-prefix)
			fast := consumedLengthParse(t, source, wire, false, nil)
			legacy := consumedLengthParse(t, source, wire, true, nil)
			consumedLengthCompare(t, fast, legacy)
			for _, run := range []consumedLengthRun{fast, legacy} {
				require.NoError(t, run.err)
				consumedLengthRequireLeaf(t, run, "@Message.Line", [2]uint64{uint64(prefix), uint64(prefix + 8)}, "A", 16)
				consumedLengthRequireLeaf(t, run, "@Message.Tail", [2]uint64{uint64(prefix + 16), 24}, uint8(tail), uint64(8-prefix))
				require.Equal(t, uint64(24), stream_parser.CalcNodeConsumedLength(run.message))
				require.Equal(t, wire, run.root.Ctx.GetItem("buffer").(*bytes.Buffer).Bytes())
			}
		})
	}
	const optional = `
Package:
  Message:
    Line:
      type: string
      length: 16
      del: "\n"
      delimiter-optional: true
    Tail: uint8
`
	for _, wire := range [][]byte{[]byte("ABZ"), []byte("A\nZ")} {
		fast := consumedLengthParse(t, optional, wire, false, nil)
		legacy := consumedLengthParse(t, optional, wire, true, nil)
		consumedLengthCompare(t, fast, legacy)
		for _, run := range []consumedLengthRun{fast, legacy} {
			require.NoError(t, run.err)
			value := strings.TrimSuffix(string(wire[:2]), "\n")
			consumedLengthRequireLeaf(t, run, "@Message.Line", [2]uint64{0, uint64(len(value) * 8)}, value, 16)
			consumedLengthRequireLeaf(t, run, "@Message.Tail", [2]uint64{16, 24}, uint8('Z'), 8)
		}
	}
}

func TestConsumedLengthDifferentialLengthOverrides(t *testing.T) {
	for _, tc := range []struct {
		name, source string
		wire         []byte
		setup        func(*base.Node)
		failure      string
	}{
		{
			name: "open stream gains bounded zero then bounded data",
			source: `
Package:
  Message:
    operator: |
      field = this.GetSubNode("Body")
      if field.HasMaxLength() { panic("open stream unexpectedly bounded") }
      field.SetMaxLength(0)
      if !field.HasMaxLength() || field.GetMaxLength() != 0 { panic("bounded zero became open") }
      field.SetMaxLength(2)
      field.Process()
      this.GetSubNode("Tail").SetMaxLength(1)
      this.ProcessSubNode("Tail")
    Body: raw
    Tail: raw
`,
			wire:  []byte{0x11, 0x22, 0x33},
			setup: func(root *base.Node) { root.Cfg.DeleteItem(base.CfgLength) },
		},
		{
			name: "live SetMaxLength zero and replacement",
			source: `
Package:
  Message:
    operator: |
      field = this.GetSubNode("Body")
      field.SetMaxLength(0)
      if !field.HasMaxLength() || field.GetMaxLength() != 0 { panic("lost bounded zero") }
      field.SetMaxLength(2)
      if field.GetMaxLength() != 2 { panic("stale explicit length") }
      field.Process()
      if this.GetSubNode("Tail").GetMaxLength() != 1 { panic("stale preceding consumption") }
      this.ProcessSubNode("Tail")
    Body: raw
    Tail: raw
`,
			wire: []byte{0x11, 0x22, 0x33},
		},
		{
			name: "length scope starts after header",
			source: `
Package:
  Message:
    length: 24
    length-for-start-field: Body
    Header: uint8
    Body: raw,2
    Tail: raw
`,
			wire: []byte{0x11, 0x22, 0x33, 0x44},
		},
		{
			name: "explicit parent cache changes through exposed map",
			source: `
Package:
  Message:
    operator: |
      this.ProcessSubNode("Header")
      if this.GetSubNode("Body").GetMaxLength() != 3 { panic("initial explicit parent cache") }
      cache = this.GetCfg("length-cache-map")
      cache["Body"] = uint64(24)
      if this.GetSubNode("Body").GetMaxLength() != 2 { panic("stale exposed parent cache") }
      this.ProcessSubNode("Body")
      this.ProcessSubNode("Tail")
    Header: uint8
    Body: raw
    Tail: uint8
`,
			wire: []byte{0x11, 0x22, 0x33, 0x44},
			setup: func(root *base.Node) {
				root.Children[0].Children[0].Cfg.SetItem(stream_parser.CfgLengthCacheMap, map[string]uint64{"Header": 32, "Body": 32, "Tail": 32})
			},
		},
		{
			name: "fixed child must not bypass ancestor boundary",
			source: `
Package:
  Message:
    length: 8
    Header: uint8
    Body: uint8
`,
			wire: []byte{0x11, 0x22}, failure: "over max size 0",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fast := consumedLengthParse(t, tc.source, tc.wire, false, tc.setup)
			legacy := consumedLengthParse(t, tc.source, tc.wire, true, tc.setup)
			consumedLengthCompare(t, fast, legacy)
			for _, run := range []consumedLengthRun{fast, legacy} {
				if tc.failure != "" {
					require.ErrorContains(t, run.err, tc.failure)
					continue
				}
				require.NoError(t, run.err)
				require.Zero(t, run.input.Len())
				require.Equal(t, tc.wire, run.root.Ctx.GetItem("buffer").(*bytes.Buffer).Bytes())
			}
		})
	}
}

func TestConsumedLengthDifferentialLiveResultAndChildrenMutation(t *testing.T) {
	const source = `
Package:
  Message:
    Line:
      type: string
      del: "\n"
    Tail: raw
`
	runs := []consumedLengthRun{
		consumedLengthParse(t, source, []byte("A\nZ"), false, nil),
		consumedLengthParse(t, source, []byte("A\nZ"), true, nil),
	}
	for _, run := range runs {
		require.NoError(t, run.err)
	}
	for _, step := range []struct {
		name      string
		mutate    func(consumedLengthRun)
		consumed  uint64
		remaining uint64
	}{
		{"original delimiter consumption", func(consumedLengthRun) {}, 24, 8},
		{"delete consumed framing override", func(run consumedLengthRun) {
			run.message.Children[0].Cfg.DeleteItem(stream_parser.CfgConsumedBits)
		}, 16, 16},
		{"replace result interval", func(run consumedLengthRun) {
			run.message.Children[0].Cfg.SetItem(base.CfgNodeResult, [2]uint64{0, 16})
		}, 24, 8},
		{"delete preceding result", func(run consumedLengthRun) {
			run.message.Children[0].Cfg.DeleteItem(base.CfgNodeResult)
		}, 8, 24},
		{"restore result and framing", func(run consumedLengthRun) {
			line := run.message.Children[0]
			line.Cfg.SetItem(base.CfgNodeResult, [2]uint64{0, 8})
			line.Cfg.SetItem(stream_parser.CfgConsumedBits, uint64(16))
		}, 24, 8},
		{"replace child without changing child count", func(run consumedLengthRun) {
			replacement := run.message.Children[0].Copy()
			replacement.Cfg.DeleteItem(base.CfgNodeResult)
			replacement.Cfg.DeleteItem(stream_parser.CfgConsumedBits)
			replacement.Cfg.SetItem(base.CfgParent, run.message)
			run.message.Children[0] = replacement
		}, 8, 24},
		{"restore replacement and reverse same child slice", func(run consumedLengthRun) {
			line := run.message.Children[0]
			line.Cfg.SetItem(base.CfgNodeResult, [2]uint64{0, 8})
			line.Cfg.SetItem(stream_parser.CfgConsumedBits, uint64(16))
			run.message.Children[0], run.message.Children[1] = run.message.Children[1], run.message.Children[0]
		}, 24, 24},
	} {
		t.Run(step.name, func(t *testing.T) {
			for _, run := range runs {
				step.mutate(run)
				require.Len(t, run.message.Children, 2)
				require.Equal(t, step.consumed, stream_parser.CalcNodeConsumedLength(run.message))
				tail := base.GetNodeByPath(run.root, "@Message.Tail")
				require.Equal(t, step.remaining, stream_parser.ConvertToYakNode(tail, nil).GetMaxLength("bit"))
			}
			consumedLengthCompare(t, runs[0], runs[1])
		})
	}
}

func TestConsumedLengthDifferentialNestedProbeRecovery(t *testing.T) {
	for _, failure := range []bool{false, true} {
		t.Run(fmt.Sprintf("inner-failure-%t", failure), func(t *testing.T) {
			last := ""
			if failure {
				last = "      this.ProcessSubNode(\"TooLong\")\n"
			}
			source := `
Package:
  Message:
    operator: |
      this.ProcessSubNode("Prefix")
      result, outer = this.TryProcessByType("Outer")
      outer.Recovery()
      if this.Length("bit") != 3 { panic("rolled-back child still consumes space") }
      this.ProcessSubNode("Payload")
    Prefix: uint8,3bit
    Payload: raw,13bit
Outer:
  operator: |
    result, inner = this.TryProcessByType("Inner")
    if !inner.OK { panic("inner probe should succeed") }
    inner.Save()
    if this.Length("bit") != 5 { panic("inner committed consumption missing") }
` + last + `
  TooLong: raw,8
Inner:
  Value: uint8,5bit
`
			wire := []byte{0xad, 0x73}
			fast := consumedLengthParse(t, source, wire, false, nil)
			legacy := consumedLengthParse(t, source, wire, true, nil)
			consumedLengthCompare(t, fast, legacy)
			for _, run := range []consumedLengthRun{fast, legacy} {
				require.NoError(t, run.err)
				require.Equal(t, wire, run.root.Ctx.GetItem("buffer").(*bytes.Buffer).Bytes())
				require.Equal(t, uint64(16), stream_parser.CalcNodeConsumedLength(run.message))
				require.Len(t, run.message.Children, 2)
				require.ErrorContains(t, run.reader.Recovery(), "no backup")
				require.ErrorContains(t, run.reader.PopBackup(), "no backup")
				_, err := run.reader.ReadBits(1)
				require.ErrorIs(t, err, io.EOF, "recovery duplicated input")
			}
		})
	}
}
