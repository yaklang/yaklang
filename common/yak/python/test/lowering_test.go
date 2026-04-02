package test

import "testing"

func TestPython_Lowering_WalrusAssignment(t *testing.T) {
	CheckPythonPrintlnValue(`
if value := 7:
    println(value)
println(value)
`, []string{"7", "7"}, t)
}

func TestPython_Lowering_AnnotatedAssignment(t *testing.T) {
	CheckPythonPrintlnValue(`
value: int = 3
println(value)
`, []string{"3"}, t)
}

func TestPython_Lowering_WithAsBinding(t *testing.T) {
	CheckPythonPrintlnValue(`
ctx = "demo"
with ctx as handle:
    println(handle)
println(handle)
`, []string{"\"demo\"", "\"demo\""}, t)
}

func TestPython_Lowering_TypeAliasAndTypeParamsCompile(t *testing.T) {
	CheckAllPythonCode(`
type UserId = int

def identity[T](value: T) -> T:
    return value

user_id: UserId = identity(1)
println(user_id)
`, t)
}

func TestPython_Lowering_ImportAliasBinding(t *testing.T) {
	CheckPythonPrintlnValue(`
import requests as req
println(req)
`, []string{"Undefined-requests"}, t)
}

func TestPython_Lowering_FromImportAliasBinding(t *testing.T) {
	CheckPythonPrintlnValue(`
from pkg.sub import run as execute
println(execute)
`, []string{"Undefined-pkg.sub.run"}, t)
}

func TestPython_Lowering_FromImportStarBinding(t *testing.T) {
	CheckPythonPrintlnValue(`
from pkg.sub import *
println(run)
`, []string{"Undefined-pkg.sub.run"}, t)
}

func TestPython_Lowering_PrintStatement(t *testing.T) {
	CheckPythonPrintlnValue(`
print 7
`, []string{"7"}, t)
}

func TestPython_Lowering_DelStatement(t *testing.T) {
	CheckPythonPrintlnValue(`
value = 3
del value
println(value)
`, []string{"Undefined-value"}, t)
}

func TestPython_Lowering_ExecStatementCompile(t *testing.T) {
	CheckAllPythonCode(`
exec "value = 1" in scope, scope
`, t)
}

func TestPython_Lowering_YieldStatementCompile(t *testing.T) {
	CheckAllPythonCode(`
def numbers():
    yield 1
    yield from items
`, t)
}

func TestPython_Lowering_AssertCompile(t *testing.T) {
	CheckAllPythonCode(`
value = 1
assert value, "value should be truthy"
println(value)
`, t)
}

func TestPython_Lowering_BreakContinue(t *testing.T) {
	CheckPythonPrintlnValue(`
i = 0
while i < 5:
    i += 1
    if i == 2:
        continue
    if i == 4:
        break
    println(i)
`, []string{"1", "3"}, t)
}

func TestPython_Lowering_TryRaiseCompile(t *testing.T) {
	CheckAllPythonCode(`
try:
    raise "boom"
except:
    handled = 1
finally:
    cleanup = 2
`, t)
}

func TestPython_Lowering_TryElseNoException(t *testing.T) {
	CheckPythonPrintlnValueContain(`
try:
    println("body")
except:
    println("except")
else:
    println("else")
finally:
    println("finally")
`, []string{"\"body\"", "\"else\"", "\"finally\""}, t)
}

func TestPython_Lowering_TryElseOnRaise(t *testing.T) {
	CheckPythonPrintlnValue(`
try:
    raise "boom"
except:
    println("except")
else:
    println("else")
finally:
    println("finally")
`, []string{"\"except\"", "\"finally\""}, t)
}

func TestPython_Lowering_TryExceptNamedValueTransport(t *testing.T) {
	CheckPythonPrintlnValue(`
try:
    raise "boom"
except ValueError as err:
    println(err)
`, []string{"\"boom\""}, t)
}

func TestPython_Lowering_TryExceptStaticTypeSelection(t *testing.T) {
	CheckPythonPrintlnValueContain(`
try:
    raise TypeError("boom")
except ValueError as err:
    println("wrong")
except TypeError as err:
    println(err)
`, []string{"TypeError(\"boom\")"}, t)
}

func TestPython_Lowering_TryExceptTupleStaticTypeSelection(t *testing.T) {
	CheckPythonPrintlnValueContain(`
try:
    raise TypeError("boom")
except (ValueError, KeyError) as err:
    println("wrong")
except (TypeError, OverflowError) as err:
    println(err)
`, []string{"TypeError(\"boom\")"}, t)
}

func TestPython_Lowering_ForRangeBreakContinue(t *testing.T) {
	CheckPythonPrintlnValue(`
for i in range(1, 6):
    if i == 2:
        continue
    if i == 4:
        break
    println(i)
`, []string{"1", "3"}, t)
}

func TestPython_Lowering_ForIterableValues(t *testing.T) {
	CheckPythonPrintlnValue(`
for item in [1, 2]:
    println(item)
`, []string{"1", "2"}, t)
}

func TestPython_Lowering_ForDictIteratesKeys(t *testing.T) {
	CheckPythonPrintlnValueContain(`
for item in {"first": 1, "second": 2}:
    println(item)
`, []string{"\"first\"", "\"second\""}, t)
}

func TestPython_Lowering_ForDictItemsDestructuring(t *testing.T) {
	CheckPythonPrintlnValueContain(`
for key, value in {"first": 1, "second": 2}.items():
    println(key)
    println(value)
`, []string{"\"first\"", "\"second\"", "1", "2"}, t)
}

func TestPython_Lowering_ForDictValuesIteration(t *testing.T) {
	CheckPythonPrintlnValueContain(`
for value in {"first": 1, "second": 2}.values():
    println(value)
`, []string{"1", "2"}, t)
}

func TestPython_Lowering_ForDictKeysIterationMethod(t *testing.T) {
	CheckPythonPrintlnValueContain(`
for key in {"first": 1, "second": 2}.keys():
    println(key)
`, []string{"\"first\"", "\"second\""}, t)
}

func TestPython_Lowering_ForSetLiteralIteration(t *testing.T) {
	CheckPythonPrintlnValueContain(`
for item in {"beta", "alpha"}:
    println(item)
`, []string{"\"alpha\"", "\"beta\""}, t)
}

func TestPython_Lowering_ListComprehension(t *testing.T) {
	CheckPythonPrintlnValueContain(`
values = [item for item in [1, 2, 3] if item != 2]
for value in values:
    println(value)
`, []string{"1", "3"}, t)
}

func TestPython_Lowering_SetComprehension(t *testing.T) {
	CheckPythonPrintlnValueContain(`
values = {item for item in [3, 2, 3, 1] if item != 2}
for value in values:
    println(value)
`, []string{"1", "3"}, t)
}

func TestPython_Lowering_DictComprehension(t *testing.T) {
	CheckPythonPrintlnValueContain(`
values = {item: item + 1 for item in [1, 2]}
for key, value in values.items():
    println(key)
    println(value)
`, []string{"1", "2", "3"}, t)
}

func TestPython_Lowering_ForTupleDestructuring(t *testing.T) {
	CheckPythonPrintlnValue(`
for left, right in [(1, 2), (3, 4)]:
    println(left)
    println(right)
`, []string{"1", "2", "3", "4"}, t)
}

func TestPython_Lowering_ForStarDestructuring(t *testing.T) {
	CheckPythonPrintlnValueContain(`
for head, tail, *rest in [(1, 2, 3, 4)]:
    println(head)
    println(tail)
    println(rest)
`, []string{"1", "2", "make([]any)"}, t)
}

func TestPython_Lowering_ForDynamicStarDestructuring(t *testing.T) {
	CheckPythonPrintlnValueContain(`
def handle(items):
    for head, *rest in items:
        println(head)
        println(rest)

handle([(1, 2, 3, 4)])
`, []string{"make(any)"}, t)
}

func TestPython_Lowering_MatchCaseLiteralOrWildcard(t *testing.T) {
	CheckPythonPrintlnValueContain(`
value = 2
match value:
    case 1:
        println("one")
    case 2 | 3:
        println("many")
    case _:
        println("other")
`, []string{"\"many\""}, t)
}

func TestPython_Lowering_MatchCaseGuard(t *testing.T) {
	CheckPythonPrintlnValueContain(`
value = 2
allow = True
match value:
    case 2 if allow:
        println("guarded")
    case _:
        println("other")
`, []string{"\"guarded\""}, t)
}

func TestPython_Lowering_MatchCaseSequenceCapture(t *testing.T) {
	CheckPythonPrintlnValue(`
match ("demo", ".", "py"):
    case filename, ".", "py":
        println(filename)
    case _:
        println("other")
`, []string{"\"demo\""}, t)
}

func TestPython_Lowering_MatchCaseSequenceCaptureGuard(t *testing.T) {
	CheckPythonPrintlnValue(`
match ("demo", ".", "py"):
    case filename, ".", "py" if filename.isidentifier():
        println(filename)
    case _:
        println("other")
`, []string{"\"demo\""}, t)
}

func TestPython_Lowering_MatchCaseSequenceCaptureGuardFallback(t *testing.T) {
	CheckPythonPrintlnValue(`
match ("demo-file", ".", "py"):
    case filename, ".", "py" if filename.isidentifier():
        println(filename)
    case _:
        println("other")
`, []string{"\"other\""}, t)
}

func TestPython_Lowering_MatchCaseDynamicSequenceCapture(t *testing.T) {
	CheckPythonPrintlnValue(`
name = "demo.py"
match name.partition("."):
    case filename, ".", "py" if filename.isidentifier():
        println(filename)
    case _:
        println(name)
`, []string{"\"demo\""}, t)
}

func TestPython_Lowering_MatchCaseStaticStarPattern(t *testing.T) {
	CheckPythonPrintlnValueContain(`
match (1, 2, 3, 4):
    case head, *rest:
        println(head)
        println(rest)
    case _:
        println("other")
`, []string{"1", "make([]any)"}, t)
}

func TestPython_Lowering_MatchCaseDynamicStarPattern(t *testing.T) {
	CheckPythonPrintlnValueContain(`
def handle(data):
    match data:
        case head, *rest:
            println(head)
            println(rest)
        case _:
            println("other")

handle((1, 2, 3, 4))
`, []string{"ParameterMember-parameter[0].0", "make(any)"}, t)
}
