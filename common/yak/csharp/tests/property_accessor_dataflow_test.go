package tests

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
)

func requireCSharpSourceReachesSink(t *testing.T, prog *ssaapi.Program, rule string) {
	t.Helper()
	result, err := prog.SyntaxFlowWithError(rule)
	require.NoError(t, err)
	sources := result.GetValues("source")
	origins := result.GetValues("origin")
	require.NotEmpty(t, sources, "source query must select the caller value")
	require.NotEmpty(t, origins, "sink argument must have a traced origin")

	sourceIDs := make(map[int64]struct{}, len(sources))
	for _, source := range sources {
		sourceIDs[source.GetId()] = struct{}{}
	}
	for _, origin := range origins {
		if _, ok := sourceIDs[origin.GetId()]; ok {
			return
		}
	}
	require.Failf(t, "source did not reach sink", "sources=%v origins=%v", sources, origins)
}

func requireCSharpCallStoredAtMember(t *testing.T, prog *ssaapi.Program, sourceName, memberKey string) {
	t.Helper()
	for _, source := range prog.Ref(sourceName) {
		for _, user := range source.GetUsers() {
			if !user.IsCall() {
				continue
			}
			for _, pair := range user.GetObjectKeyPairs() {
				if len(pair) == 2 && strings.Trim(pair[1].String(), `"`) == memberKey {
					return
				}
			}
		}
	}
	require.Failf(t, "assignment was not retained as member storage",
		"source=%s member=%s", sourceName, memberKey)
}

func requireCSharpSingleCall(t *testing.T, values ssaapi.Values, description string) *ssaapi.Value {
	t.Helper()
	calls := values.Filter(func(value *ssaapi.Value) bool {
		return value != nil && value.GetOpcode() == "Call"
	})
	require.Len(t, calls, 1, description+": %s", calls)
	return calls[0]
}

func requireCSharpNamedCall(t *testing.T, prog *ssaapi.Program, name string) *ssaapi.Value {
	t.Helper()
	return requireCSharpSingleCall(t, prog.Ref(name).GetUsers(), "expected exactly one call to "+name)
}

func requireCSharpCallReceiver(t *testing.T, call, receiver *ssaapi.Value) {
	t.Helper()
	require.NotNil(t, call)
	require.NotNil(t, receiver)
	actual := call.GetOperand(1)
	require.NotNil(t, actual, "instance call must contain a receiver: %s", call)
	require.Equal(t, receiver.GetId(), actual.GetId(), "wrong instance receiver for %s", call)
}

func TestCSharp_OOP_PropertyAccessorDataFlow(t *testing.T) {
	prog := parseCSharpSemantics(t, `
public class AccessorBox {
    public string Value {
        get { return propertySource(); }
        set { propertySetterSink(value); }
    }

    public static string StaticValue {
        get { return staticPropertySource(); }
        set { staticPropertySetterSink(value); }
    }
}

public class Program {
    public static void Main(string[] args) {
        var box = new AccessorBox();
        propertyReadSink(box.Value);
        box.Value = propertyCallerSource();
        staticPropertyReadSink(AccessorBox.StaticValue);
        AccessorBox.StaticValue = staticPropertyCallerSource();
    }
}`)

	requireCSharpSourceReachesSink(t, prog,
		`propertySource as $source; propertyReadSink(* #-> as $origin)`)
	requireCSharpSourceReachesSink(t, prog,
		`propertyCallerSource as $source; propertySetterSink(* #-> as $origin)`)
	requireCSharpSourceReachesSink(t, prog,
		`staticPropertySource as $source; staticPropertyReadSink(* #-> as $origin)`)
	requireCSharpSourceReachesSink(t, prog,
		`staticPropertyCallerSource as $source; staticPropertySetterSink(* #-> as $origin)`)
	requireCSharpCallStoredAtMember(t, prog, "propertyCallerSource", "Value")
	requireCSharpCallStoredAtMember(t, prog, "staticPropertyCallerSource", "StaticValue")
}

func TestCSharp_OOP_IndexerAccessorDataFlow(t *testing.T) {
	prog := parseCSharpSemantics(t, `
public class IndexedBox {
    public string this[int index] {
        get { return indexerSource(index); }
        set { indexerSetterSink(value); }
    }
}

public class Program {
    public static void Main(string[] args) {
        var box = new IndexedBox();
        indexerReadSink(box[7]);
        box[7] = indexerCallerSource();
    }
}`)

	requireCSharpSourceReachesSink(t, prog,
		`indexerSource as $source; indexerReadSink(* #-> as $origin)`)
	requireCSharpSourceReachesSink(t, prog,
		`indexerCallerSource as $source; indexerSetterSink(* #-> as $origin)`)
	requireCSharpCallStoredAtMember(t, prog, "indexerCallerSource", "7")
}

func TestCSharp_OOP_InstanceMethodAndAccessorKeepPerCallReceiver(t *testing.T) {
	prog := parseCSharpSemantics(t, `
public class ReceiverBox {
    private string backing;

    public void Put(string value) { backing = value; }
    public string Value {
        get { return backing; }
        set { backing = value; }
    }
}

public class Program {
    public static void Main(string[] args) {
        var first = new ReceiverBox();
        var second = new ReceiverBox();

        first.Put(firstMethodSource());
        second.Put(secondMethodSource());
        first.Value = firstPropertySource();
        second.Value = secondPropertySource();
        firstPropertyReadSink(first.Value);
        secondPropertyReadSink(second.Value);
    }
}`)

	firstValues := prog.Ref("first")
	secondValues := prog.Ref("second")
	require.Len(t, firstValues, 1)
	require.Len(t, secondValues, 1)
	first, second := firstValues[0], secondValues[0]

	firstMethodSource := requireCSharpNamedCall(t, prog, "firstMethodSource")
	secondMethodSource := requireCSharpNamedCall(t, prog, "secondMethodSource")
	firstMethodCall := requireCSharpSingleCall(t, firstMethodSource.GetUsers(), "first.Put must be emitted once")
	secondMethodCall := requireCSharpSingleCall(t, secondMethodSource.GetUsers(), "second.Put must be emitted once")
	requireCSharpCallReceiver(t, firstMethodCall, first)
	requireCSharpCallReceiver(t, secondMethodCall, second)

	firstPropertySource := requireCSharpNamedCall(t, prog, "firstPropertySource")
	secondPropertySource := requireCSharpNamedCall(t, prog, "secondPropertySource")
	firstSetterCall := requireCSharpSingleCall(t, firstPropertySource.GetUsers(), "first.Value setter must be emitted once")
	secondSetterCall := requireCSharpSingleCall(t, secondPropertySource.GetUsers(), "second.Value setter must be emitted once")
	requireCSharpCallReceiver(t, firstSetterCall, first)
	requireCSharpCallReceiver(t, secondSetterCall, second)

	firstRead := requireCSharpNamedCall(t, prog, "firstPropertyReadSink").GetOperand(1)
	secondRead := requireCSharpNamedCall(t, prog, "secondPropertyReadSink").GetOperand(1)
	require.NotNil(t, firstRead)
	require.NotNil(t, secondRead)
	require.Equal(t, "Call", firstRead.GetOpcode())
	require.Equal(t, "Call", secondRead.GetOpcode())
	requireCSharpCallReceiver(t, firstRead, first)
	requireCSharpCallReceiver(t, secondRead, second)
}

func TestCSharp_OOP_MethodGroupsKeepPerDelegateReceiver(t *testing.T) {
	prog := parseCSharpSemantics(t, `
using System;

public class ReceiverBox {
    public void Put(string value) { consume(value); }
}

public class Program {
    public static void Main(string[] args) {
        var first = new ReceiverBox();
        var second = new ReceiverBox();
        Action<string> firstPut = first.Put;
        Action<string> secondPut = second.Put;
        firstPut(firstMethodGroupSource());
        secondPut(secondMethodGroupSource());
    }
}`)

	firstValues := prog.Ref("first")
	secondValues := prog.Ref("second")
	require.Len(t, firstValues, 1)
	require.Len(t, secondValues, 1)

	firstSource := requireCSharpNamedCall(t, prog, "firstMethodGroupSource")
	secondSource := requireCSharpNamedCall(t, prog, "secondMethodGroupSource")
	firstCall := requireCSharpSingleCall(t, firstSource.GetUsers(), "first method group must be invoked once")
	secondCall := requireCSharpSingleCall(t, secondSource.GetUsers(), "second method group must be invoked once")
	requireCSharpCallReceiver(t, firstCall, firstValues[0])
	requireCSharpCallReceiver(t, secondCall, secondValues[0])
}

func TestCSharp_OOP_DelegateFieldIsNotBoundAsInstanceMethod(t *testing.T) {
	prog := parseCSharpSemantics(t, `
using System;

public class CallbackHolder {
    public Action<string> Callback;
}

public class Program {
    public static void Main(Action<string> callback) {
        var holder = new CallbackHolder();
        holder.Callback = callback;
        holder.Callback(delegateFieldSource());
    }
}`)

	source := requireCSharpNamedCall(t, prog, "delegateFieldSource")
	invocation := requireCSharpSingleCall(t, source.GetUsers(), "delegate field must be invoked once")
	require.Len(t, invocation.GetOperands(), 2, "delegate field call must contain only callee and explicit argument")
	require.Equal(t, source.GetId(), invocation.GetOperand(1).GetId(),
		"delegate field call must not inject the containing object as a receiver")
}

func TestCSharp_OOP_MultiParameterIndexerPreservesReceiverAndArguments(t *testing.T) {
	prog := parseCSharpSemantics(t, `
public class Matrix {
    public string this[int row, int column] {
        get { return indexerReadSource(row, column); }
        set { indexerSetterSink(row, column, value); }
    }
}

public class Program {
    public static void Main(string[] args) {
        var matrix = new Matrix();
        indexerReadSink(matrix[readRowSource(), readColumnSource()]);
        matrix[writeRowSource(), writeColumnSource()] = indexerValueSource();
    }
}`)

	matrixValues := prog.Ref("matrix")
	require.Len(t, matrixValues, 1)
	matrix := matrixValues[0]

	readRow := requireCSharpNamedCall(t, prog, "readRowSource")
	readColumn := requireCSharpNamedCall(t, prog, "readColumnSource")
	getter := requireCSharpNamedCall(t, prog, "indexerReadSink").GetOperand(1)
	require.NotNil(t, getter)
	require.Equal(t, "Call", getter.GetOpcode())
	require.Len(t, getter.GetOperands(), 4, "get_Item must receive method, receiver, row and column")
	requireCSharpCallReceiver(t, getter, matrix)
	require.Equal(t, readRow.GetId(), getter.GetOperand(2).GetId())
	require.Equal(t, readColumn.GetId(), getter.GetOperand(3).GetId())

	writeRow := requireCSharpNamedCall(t, prog, "writeRowSource")
	writeColumn := requireCSharpNamedCall(t, prog, "writeColumnSource")
	writeValue := requireCSharpNamedCall(t, prog, "indexerValueSource")
	setter := requireCSharpSingleCall(t, writeValue.GetUsers(), "multi-parameter set_Item must be emitted once")
	require.Len(t, setter.GetOperands(), 5, "set_Item must receive method, receiver, row, column and value")
	requireCSharpCallReceiver(t, setter, matrix)
	require.Equal(t, writeRow.GetId(), setter.GetOperand(2).GetId())
	require.Equal(t, writeColumn.GetId(), setter.GetOperand(3).GetId())
	require.Equal(t, writeValue.GetId(), setter.GetOperand(4).GetId())
}

func TestCSharp_OOP_ObjectInitializerCallsDeclaredSetterAndStoresValue(t *testing.T) {
	prog := parseCSharpSemantics(t, `
public class InitializerBox {
    public string Value {
        get { return initializerGetterSource(); }
        set { initializerSetterSink(value); }
    }
}

public class Program {
    public static void Main(string[] args) {
        var box = new InitializerBox { Value = initializerCallerSource() };
        consume(box);
    }
}`)

	requireCSharpSourceReachesSink(t, prog,
		`initializerCallerSource as $source; initializerSetterSink(* #-> as $origin)`)
	requireCSharpCallStoredAtMember(t, prog, "initializerCallerSource", "Value")

	boxValues := prog.Ref("box")
	require.Len(t, boxValues, 1)
	source := requireCSharpNamedCall(t, prog, "initializerCallerSource")
	setter := requireCSharpSingleCall(t, source.GetUsers(), "object initializer setter must be emitted once")
	requireCSharpCallReceiver(t, setter, boxValues[0])
}

func TestCSharp_OOP_VirtualPropertyDispatchUsesExactAccessorSignature(t *testing.T) {
	prog := parseCSharpSemantics(t, `
public class PropertyBase {
    public virtual object Value { get { return cleanBaseProperty(); } }
}
public class PropertyOverride : PropertyBase {
    public override object Value { get { return overridePropertySource(); } }
}
public class PropertyHide : PropertyBase {
    public new object Value { get { return hiddenPropertySource(); } }
}
public class PropertyDispatch {
    public static void Read(PropertyBase value) { propertyDispatchSink(value.Value); }
    public static void Main() {
        Read(new PropertyOverride());
        Read(new PropertyHide());
    }
}
`)

	flow, err := prog.SyntaxFlowWithError(`propertyDispatchSink(* #-> as $origin)`)
	require.NoError(t, err)
	require.Contains(t, flow.GetValues("origin").String(), "overridePropertySource")
	require.Contains(t, flow.GetValues("origin").String(), "cleanBaseProperty")
	require.NotContains(t, flow.GetValues("origin").String(), "hiddenPropertySource",
		"a new property must not enter the base accessor's virtual-dispatch graph")
}

func TestCSharp_OOP_IndexerOverloadSelectsAccessorByArgumentType(t *testing.T) {
	prog := parseCSharpSemantics(t, `
public class OverloadedIndexer {
    public object this[int index] { get { return cleanIntIndexer(); } }
    public object this[string index] { get { return stringIndexerSource(); } }
}
public class Program {
    public static void Main() { indexerOverloadSink(new OverloadedIndexer()["key"]); }
}
`)

	flow, err := prog.SyntaxFlowWithError(`indexerOverloadSink(* #-> as $origin)`)
	require.NoError(t, err)
	require.Contains(t, flow.GetValues("origin").String(), "stringIndexerSource")
	require.NotContains(t, flow.GetValues("origin").String(), "cleanIntIndexer")
}
