package bin_parser

import (
	"bytes"
	"encoding/binary"
	"os"
	"testing"

	"github.com/gopacket/gopacket/pcapgo"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
)

var (
	protocolCorpusSVBenchmarkNodeSink  *base.Node
	protocolCorpusSVBenchmarkValueSink *base.NodeValue
)

type protocolCorpusSVBenchmarkExpected struct {
	appID                 uint64
	length                uint64
	reserved1             uint64
	reserved2             uint64
	asduCount             uint64
	svID                  []byte
	sampleCounter         uint64
	configurationRevision uint64
	sampleSynchronization uint64
	sampleData            []byte
}

type protocolCorpusSVBenchmarkValues struct {
	asduCount             *base.NodeValue
	svID                  *base.NodeValue
	sampleCounter         *base.NodeValue
	configurationRevision *base.NodeValue
	sampleSynchronization *base.NodeValue
	sampleData            *base.NodeValue
	counts                [6]uint8
}

type protocolCorpusSVBenchmarkMode uint8

const (
	protocolCorpusSVParseOnly protocolCorpusSVBenchmarkMode = iota
	protocolCorpusSVParseAndRootResult
	protocolCorpusSVParseRootAndFieldResults
	protocolCorpusSVParseAndSingleRootTraversal
)

type protocolCorpusSVBenchmarkOutcome struct {
	node   *base.Node
	root   *base.NodeValue
	values protocolCorpusSVBenchmarkValues
}

// BenchmarkProtocolCorpusSVExecOut keeps all variants on the first complete,
// real mgadelha SV frame and the same bounded parser input. The initial and
// final validation are outside the timer; the named work itself remains inside
// so the sub-benchmarks separate parser, root Result, repeated field Result and
// a single traversal of the already-materialized root Result.
func BenchmarkProtocolCorpusSVExecOut(b *testing.B) {
	benchmarkProtocolCorpusSVExecOut(b, false)
}

// BenchmarkProtocolCorpusSVExecOutLegacy is a same-binary diagnostic oracle.
// It keeps the identical frame, modes and validation while explicitly forcing
// the uncached expression evaluator through the public parser context.
func BenchmarkProtocolCorpusSVExecOutLegacy(b *testing.B) {
	benchmarkProtocolCorpusSVExecOut(b, true)
}

// Keep compiled expression caching enabled on both sides, disabling only the
// pure scalar-result fast path. This isolates the current optimization from
// the earlier compilation-cache improvement.
func BenchmarkProtocolCorpusSVExecOutScalarLegacy(b *testing.B) {
	benchmarkProtocolCorpusSVExecOutWithConfig(b, map[string]any{"outScalarLegacy": true})
}

func benchmarkProtocolCorpusSVExecOut(b *testing.B, legacy bool) {
	var config map[string]any
	if legacy {
		config = map[string]any{"outProgramLegacy": true}
	}
	benchmarkProtocolCorpusSVExecOutWithConfig(b, config)
}

func benchmarkProtocolCorpusSVExecOutWithConfig(b *testing.B, config map[string]any) {
	input, expected := protocolCorpusSVBenchmarkInput(b)
	for _, benchmark := range []struct {
		name string
		mode protocolCorpusSVBenchmarkMode
	}{
		{name: "parse-only", mode: protocolCorpusSVParseOnly},
		{name: "parse-and-root-result", mode: protocolCorpusSVParseAndRootResult},
		{name: "parse-root-and-repeated-field-results", mode: protocolCorpusSVParseRootAndFieldResults},
		{name: "parse-and-single-root-result-traversal", mode: protocolCorpusSVParseAndSingleRootTraversal},
	} {
		b.Run(benchmark.name, func(b *testing.B) {
			// Warm both the rule/operator caches and any expression-program cache.
			warm := protocolCorpusSVBenchmarkRun(b, input, config, benchmark.mode)
			protocolCorpusSVBenchmarkValidateOutcome(b, warm, expected)

			b.ReportAllocs()
			b.SetBytes(int64(len(input)))
			b.ResetTimer()
			var last protocolCorpusSVBenchmarkOutcome
			for iteration := 0; iteration < b.N; iteration++ {
				last = protocolCorpusSVBenchmarkRun(b, input, config, benchmark.mode)
			}
			b.StopTimer()

			protocolCorpusSVBenchmarkValidateOutcome(b, last, expected)
			protocolCorpusSVBenchmarkNodeSink = last.node
			protocolCorpusSVBenchmarkValueSink = last.root
		})
	}
}

func protocolCorpusSVBenchmarkRun(b *testing.B, input []byte, config map[string]any, mode protocolCorpusSVBenchmarkMode) protocolCorpusSVBenchmarkOutcome {
	b.Helper()
	reader := newProtocolCorpusBoundedReader(input)
	node, err := parser.ParseBinaryWithConfig(reader, "iec61850", config, "SampledValues")
	if err != nil {
		b.Fatal(err)
	}
	if remaining := reader.Len(); remaining != 0 {
		b.Fatalf("SV parser left %d bytes, want 0", remaining)
	}
	outcome := protocolCorpusSVBenchmarkOutcome{node: node}
	if mode == protocolCorpusSVParseOnly {
		return outcome
	}
	outcome.root, err = node.Result()
	if err != nil {
		b.Fatal(err)
	}
	if outcome.root == nil {
		b.Fatal("SV root Result is nil")
	}
	switch mode {
	case protocolCorpusSVParseAndRootResult:
		return outcome
	case protocolCorpusSVParseRootAndFieldResults:
		outcome.values = protocolCorpusSVBenchmarkFieldResults(b, node)
	case protocolCorpusSVParseAndSingleRootTraversal:
		protocolCorpusSVBenchmarkCollectResult(outcome.root, &outcome.values)
	default:
		b.Fatalf("unknown SV benchmark mode %d", mode)
	}
	return outcome
}

func protocolCorpusSVBenchmarkFieldResults(b *testing.B, node *base.Node) protocolCorpusSVBenchmarkValues {
	b.Helper()
	var result protocolCorpusSVBenchmarkValues
	fields := []struct {
		name  string
		value **base.NodeValue
		index int
	}{
		{name: "ASDU Count", value: &result.asduCount, index: 0},
		{name: "SV ID", value: &result.svID, index: 1},
		{name: "Sample Counter", value: &result.sampleCounter, index: 2},
		{name: "Configuration Revision", value: &result.configurationRevision, index: 3},
		{name: "Sample Synchronization", value: &result.sampleSynchronization, index: 4},
		{name: "Sample Data", value: &result.sampleData, index: 5},
	}
	for _, field := range fields {
		target := protocolCorpusFindNode(node, field.name)
		if target == nil {
			b.Fatalf("missing SV field %q", field.name)
		}
		value, err := target.Result()
		if err != nil {
			b.Fatalf("read SV field %q: %v", field.name, err)
		}
		*field.value = value
		result.counts[field.index]++
	}
	return result
}

func protocolCorpusSVBenchmarkCollectResult(value *base.NodeValue, result *protocolCorpusSVBenchmarkValues) {
	if value == nil {
		return
	}
	switch value.Name {
	case "ASDU Count":
		result.asduCount = value
		result.counts[0]++
	case "SV ID":
		result.svID = value
		result.counts[1]++
	case "Sample Counter":
		result.sampleCounter = value
		result.counts[2]++
	case "Configuration Revision":
		result.configurationRevision = value
		result.counts[3]++
	case "Sample Synchronization":
		result.sampleSynchronization = value
		result.counts[4]++
	case "Sample Data":
		result.sampleData = value
		result.counts[5]++
	}
	for _, child := range value.Children() {
		protocolCorpusSVBenchmarkCollectResult(child, result)
	}
}

func protocolCorpusSVBenchmarkValidateOutcome(b *testing.B, outcome protocolCorpusSVBenchmarkOutcome, expected protocolCorpusSVBenchmarkExpected) {
	b.Helper()
	if outcome.node == nil {
		b.Fatal("SV parser returned a nil node")
	}
	root := outcome.root
	if root == nil {
		var err error
		root, err = outcome.node.Result()
		if err != nil {
			b.Fatalf("materialize SV root Result: %v", err)
		}
	}
	protocolCorpusSVBenchmarkValidateHeader(b, root, expected)
	values := outcome.values
	if values.asduCount == nil {
		protocolCorpusSVBenchmarkCollectResult(root, &values)
	}
	for index, count := range values.counts {
		if count != 1 {
			b.Fatalf("SV result field %d occurred %d times, want exactly 1", index, count)
		}
	}
	protocolCorpusSVBenchmarkRequireUint(b, "ASDU Count", values.asduCount, expected.asduCount)
	protocolCorpusSVBenchmarkRequireBytes(b, "SV ID", values.svID, expected.svID)
	protocolCorpusSVBenchmarkRequireUint(b, "Sample Counter", values.sampleCounter, expected.sampleCounter)
	protocolCorpusSVBenchmarkRequireUint(b, "Configuration Revision", values.configurationRevision, expected.configurationRevision)
	protocolCorpusSVBenchmarkRequireUint(b, "Sample Synchronization", values.sampleSynchronization, expected.sampleSynchronization)
	protocolCorpusSVBenchmarkRequireBytes(b, "Sample Data", values.sampleData, expected.sampleData)
	if len(expected.sampleData) != 64 {
		b.Fatalf("SV Sample Data has %d bytes, want 64", len(expected.sampleData))
	}
}

func protocolCorpusSVBenchmarkValidateHeader(b *testing.B, root *base.NodeValue, expected protocolCorpusSVBenchmarkExpected) {
	b.Helper()
	var values struct {
		appID     *base.NodeValue
		length    *base.NodeValue
		reserved1 *base.NodeValue
		reserved2 *base.NodeValue
	}
	var walk func(*base.NodeValue)
	walk = func(value *base.NodeValue) {
		if value == nil {
			return
		}
		switch value.Name {
		case "APPID":
			values.appID = value
		case "Length":
			// Only the fixed frame Length is a uint16. BER lengths use the
			// same name but have a different concrete result type.
			if _, ok := value.Value.(uint16); ok {
				values.length = value
			}
		case "Reserved 1":
			values.reserved1 = value
		case "Reserved 2":
			values.reserved2 = value
		}
		for _, child := range value.Children() {
			walk(child)
		}
	}
	walk(root)
	protocolCorpusSVBenchmarkRequireUint(b, "APPID", values.appID, expected.appID)
	protocolCorpusSVBenchmarkRequireUint(b, "Length", values.length, expected.length)
	protocolCorpusSVBenchmarkRequireUint(b, "Reserved 1", values.reserved1, expected.reserved1)
	protocolCorpusSVBenchmarkRequireUint(b, "Reserved 2", values.reserved2, expected.reserved2)
}

func protocolCorpusSVBenchmarkRequireUint(b *testing.B, name string, value *base.NodeValue, expected uint64) {
	b.Helper()
	if value == nil {
		b.Fatalf("missing SV result %q", name)
	}
	var actual uint64
	switch number := value.Value.(type) {
	case uint8:
		actual = uint64(number)
	case uint16:
		actual = uint64(number)
	case uint32:
		actual = uint64(number)
	case uint64:
		actual = number
	case int:
		actual = uint64(number)
	default:
		b.Fatalf("SV result %q has numeric type %T", name, value.Value)
	}
	if actual != expected {
		b.Fatalf("SV result %q = %d, want %d", name, actual, expected)
	}
}

func protocolCorpusSVBenchmarkRequireBytes(b *testing.B, name string, value *base.NodeValue, expected []byte) {
	b.Helper()
	if value == nil {
		b.Fatalf("missing SV result %q", name)
	}
	var actual []byte
	switch data := value.Value.(type) {
	case []byte:
		actual = data
	case string:
		actual = []byte(data)
	default:
		b.Fatalf("SV result %q has byte type %T", name, value.Value)
	}
	if !bytes.Equal(actual, expected) {
		b.Fatalf("SV result %q = %x, want %x", name, actual, expected)
	}
}

func protocolCorpusSVBenchmarkInput(b *testing.B) ([]byte, protocolCorpusSVBenchmarkExpected) {
	b.Helper()
	file, err := os.Open("testdata/protocol-corpus/captures/mgadelha-sv/mgadelha-sv.cap")
	if err != nil {
		b.Fatal(err)
	}
	defer file.Close()
	reader, err := pcapgo.NewReader(file)
	if err != nil {
		b.Fatal(err)
	}
	frame, _, err := reader.ReadPacketData()
	if err != nil {
		b.Fatal(err)
	}
	if len(frame) < 28 {
		b.Fatalf("SV frame has %d bytes, want at least 28", len(frame))
	}
	if value := binary.BigEndian.Uint16(frame[12:14]); value != 0x8100 {
		b.Fatalf("SV frame VLAN EtherType = 0x%04x, want 0x8100", value)
	}
	if value := binary.BigEndian.Uint16(frame[16:18]); value != 0x88ba {
		b.Fatalf("SV frame inner EtherType = 0x%04x, want 0x88ba", value)
	}
	input := append([]byte(nil), frame[18:]...)
	length := int(binary.BigEndian.Uint16(input[2:4]))
	if length != len(input) {
		b.Fatalf("SV declared length %d, captured parser input %d", length, len(input))
	}
	expected := protocolCorpusSVBenchmarkExpected{
		appID:     uint64(binary.BigEndian.Uint16(input[0:2])),
		length:    uint64(length),
		reserved1: uint64(binary.BigEndian.Uint16(input[4:6])),
		reserved2: uint64(binary.BigEndian.Uint16(input[6:8])),
	}
	outer := protocolCorpusSVBenchmarkBERFields(b, input[8:])
	if len(outer) != 1 || outer[0].tag != 0x60 {
		b.Fatalf("SV outer BER fields = %#v, want one tag 0x60", outer)
	}
	pdu := protocolCorpusSVBenchmarkBERFields(b, outer[0].value)
	if len(pdu) != 2 || pdu[0].tag != 0x80 || pdu[1].tag != 0xa2 {
		b.Fatalf("SV PDU has unexpected fields")
	}
	expected.asduCount = protocolCorpusSVBenchmarkBERUnsigned(b, pdu[0].value)
	asdus := protocolCorpusSVBenchmarkBERFields(b, pdu[1].value)
	if expected.asduCount != 1 || len(asdus) != 1 || asdus[0].tag != 0x30 {
		b.Fatalf("SV first frame has ASDU count %d and %d sequence entries", expected.asduCount, len(asdus))
	}
	fields := protocolCorpusSVBenchmarkBERFields(b, asdus[0].value)
	wantTags := []byte{0x80, 0x82, 0x83, 0x85, 0x87}
	if len(fields) != len(wantTags) {
		b.Fatalf("SV ASDU has %d fields, want %d", len(fields), len(wantTags))
	}
	for index, tag := range wantTags {
		if fields[index].tag != tag {
			b.Fatalf("SV ASDU field %d tag = 0x%02x, want 0x%02x", index, fields[index].tag, tag)
		}
	}
	expected.svID = append([]byte(nil), fields[0].value...)
	expected.sampleCounter = protocolCorpusSVBenchmarkBERUnsigned(b, fields[1].value)
	expected.configurationRevision = protocolCorpusSVBenchmarkBERUnsigned(b, fields[2].value)
	expected.sampleSynchronization = protocolCorpusSVBenchmarkBERUnsigned(b, fields[3].value)
	expected.sampleData = append([]byte(nil), fields[4].value...)
	return input, expected
}

type protocolCorpusSVBenchmarkBERField struct {
	tag   byte
	value []byte
}

func protocolCorpusSVBenchmarkBERFields(b *testing.B, input []byte) []protocolCorpusSVBenchmarkBERField {
	b.Helper()
	var fields []protocolCorpusSVBenchmarkBERField
	for offset := 0; offset < len(input); {
		if len(input)-offset < 2 {
			b.Fatalf("truncated SV BER field at byte %d", offset)
		}
		tag, first := input[offset], input[offset+1]
		offset += 2
		length := uint64(first)
		if first&0x80 != 0 {
			width := int(first & 0x7f)
			if width < 1 || width > 4 || len(input)-offset < width {
				b.Fatalf("invalid SV BER length at byte %d", offset-1)
			}
			length = 0
			for _, octet := range input[offset : offset+width] {
				length = length<<8 | uint64(octet)
			}
			offset += width
		}
		if length > uint64(len(input)-offset) {
			b.Fatalf("SV BER value length %d exceeds remaining %d", length, len(input)-offset)
		}
		fields = append(fields, protocolCorpusSVBenchmarkBERField{tag: tag, value: input[offset : offset+int(length)]})
		offset += int(length)
	}
	return fields
}

func protocolCorpusSVBenchmarkBERUnsigned(b *testing.B, input []byte) uint64 {
	b.Helper()
	if len(input) < 1 || len(input) > 8 {
		b.Fatalf("SV BER integer has %d bytes", len(input))
	}
	var result uint64
	for _, octet := range input {
		result = result<<8 | uint64(octet)
	}
	return result
}
