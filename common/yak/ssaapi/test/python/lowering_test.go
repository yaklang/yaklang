package python

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestPython_WalrusFlow(t *testing.T) {
	code := `
def sink(value):
    pass

if user_id := 7:
    sink(user_id)
`
	ssatest.CheckSyntaxFlow(t, code,
		"sink(* as $param)",
		map[string][]string{
			"param": {"7"},
		},
		ssaapi.WithLanguage(ssaconfig.PYTHON))
}

func TestPython_WithAsFlow(t *testing.T) {
	code := `
def sink(value):
    pass

ctx = "demo"
with ctx as handle:
    sink(handle)
`
	ssatest.CheckSyntaxFlow(t, code,
		"sink(* as $param)",
		map[string][]string{
			"param": {"\"demo\""},
		},
		ssaapi.WithLanguage(ssaconfig.PYTHON))
}

func TestPython_TypeAliasAndAnnotatedAssignmentFlow(t *testing.T) {
	code := `
def sink(value):
    pass

type UserId = int
user_id: UserId = 9
sink(user_id)
`
	ssatest.CheckSyntaxFlow(t, code,
		"sink(* as $param)",
		map[string][]string{
			"param": {"9"},
		},
		ssaapi.WithLanguage(ssaconfig.PYTHON))
}

func TestPython_ImportAliasFlow(t *testing.T) {
	code := `
def sink(value):
    pass

import requests as req
sink(req)
`
	ssatest.CheckSyntaxFlow(t, code,
		"sink(* as $param)",
		map[string][]string{
			"param": {"Undefined-requests"},
		},
		ssaapi.WithLanguage(ssaconfig.PYTHON))
}

func TestPython_FromImportAliasFlow(t *testing.T) {
	code := `
def sink(value):
    pass

from pkg.sub import run as execute
sink(execute)
`
	ssatest.CheckSyntaxFlow(t, code,
		"sink(* as $param)",
		map[string][]string{
			"param": {"Undefined-pkg.sub.run"},
		},
		ssaapi.WithLanguage(ssaconfig.PYTHON))
}

func TestPython_FromImportStarFlow(t *testing.T) {
	code := `
def sink(value):
    pass

from pkg.sub import *
sink(run)
`
	ssatest.CheckSyntaxFlow(t, code,
		"sink(* as $param)",
		map[string][]string{
			"param": {"Undefined-pkg.sub.run"},
		},
		ssaapi.WithLanguage(ssaconfig.PYTHON))
}

func TestPython_DelStatementFlow(t *testing.T) {
	code := `
def sink(value):
    pass

value = 3
del value
sink(value)
`
	ssatest.CheckSyntaxFlow(t, code,
		"sink(* as $param)",
		map[string][]string{
			"param": {"Undefined-value"},
		},
		ssaapi.WithLanguage(ssaconfig.PYTHON))
}

func TestPython_TypeParamSyntaxFlow(t *testing.T) {
	code := `
def identity[T](value: T) -> T:
    return value

identity("demo")
`
	ssatest.CheckSyntaxFlow(t, code,
		"identity(* #-> * as $param)",
		map[string][]string{
			"param": {"\"demo\""},
		},
		ssaapi.WithLanguage(ssaconfig.PYTHON))
}

func TestPython_WhileBreakContinueFlow(t *testing.T) {
	code := `
def sink(value):
    pass

i = 0
while i < 5:
    i += 1
    if i == 2:
        continue
    if i == 4:
        break
    sink(i)
`
	ssatest.CheckSyntaxFlow(t, code,
		"sink(* as $param)",
		map[string][]string{
			"param": {"1", "3"},
		},
		ssaapi.WithLanguage(ssaconfig.PYTHON))
}

func TestPython_ForRangeBreakContinueFlow(t *testing.T) {
	code := `
def sink(value):
    pass

for i in range(1, 6):
    if i == 2:
        continue
    if i == 4:
        break
    sink(i)
`
	ssatest.CheckSyntaxFlow(t, code,
		"sink(* as $param)",
		map[string][]string{
			"param": {"1", "3"},
		},
		ssaapi.WithLanguage(ssaconfig.PYTHON))
}

func TestPython_ForIterableValuesFlow(t *testing.T) {
	code := `
def sink(value):
    pass

for item in [1, 2]:
    sink(item)
`
	ssatest.CheckSyntaxFlow(t, code,
		"sink(* as $param)",
		map[string][]string{
			"param": {"1", "2"},
		},
		ssaapi.WithLanguage(ssaconfig.PYTHON))
}

func TestPython_ForDictIteratesKeysFlow(t *testing.T) {
	code := `
def sink(value):
    pass

for item in {"first": 1, "second": 2}:
    sink(item)
`
	ssatest.CheckSyntaxFlowContain(t, code,
		"sink(* as $param)",
		map[string][]string{
			"param": {"\"first\"", "\"second\""},
		},
		ssaapi.WithLanguage(ssaconfig.PYTHON))
}

func TestPython_ForDictItemsDestructuringFlow(t *testing.T) {
	code := `
def sink(value):
    pass

for key, value in {"first": 1, "second": 2}.items():
    sink(key)
    sink(value)
`
	ssatest.CheckSyntaxFlowContain(t, code,
		"sink(* as $param)",
		map[string][]string{
			"param": {"\"first\"", "\"second\"", "1", "2"},
		},
		ssaapi.WithLanguage(ssaconfig.PYTHON))
}

func TestPython_ForDictValuesIterationFlow(t *testing.T) {
	code := `
def sink(value):
    pass

for value in {"first": 1, "second": 2}.values():
    sink(value)
`
	ssatest.CheckSyntaxFlowContain(t, code,
		"sink(* as $param)",
		map[string][]string{
			"param": {"1", "2"},
		},
		ssaapi.WithLanguage(ssaconfig.PYTHON))
}

func TestPython_ForDictKeysIterationMethodFlow(t *testing.T) {
	code := `
def sink(value):
    pass

for key in {"first": 1, "second": 2}.keys():
    sink(key)
`
	ssatest.CheckSyntaxFlowContain(t, code,
		"sink(* as $param)",
		map[string][]string{
			"param": {"\"first\"", "\"second\""},
		},
		ssaapi.WithLanguage(ssaconfig.PYTHON))
}

func TestPython_ForSetLiteralIterationFlow(t *testing.T) {
	code := `
def sink(value):
    pass

for item in {"beta", "alpha"}:
    sink(item)
`
	ssatest.CheckSyntaxFlowContain(t, code,
		"sink(* as $param)",
		map[string][]string{
			"param": {"\"alpha\"", "\"beta\""},
		},
		ssaapi.WithLanguage(ssaconfig.PYTHON))
}

func TestPython_ListComprehensionFlow(t *testing.T) {
	code := `
def sink(value):
    pass

values = [item for item in [1, 2, 3] if item != 2]
for value in values:
    sink(value)
`
	ssatest.CheckSyntaxFlowContain(t, code,
		"sink(* as $param)",
		map[string][]string{
			"param": {"1", "3"},
		},
		ssaapi.WithLanguage(ssaconfig.PYTHON))
}

func TestPython_SetComprehensionFlow(t *testing.T) {
	code := `
def sink(value):
    pass

values = {item for item in [3, 2, 3, 1] if item != 2}
for value in values:
    sink(value)
`
	ssatest.CheckSyntaxFlowContain(t, code,
		"sink(* as $param)",
		map[string][]string{
			"param": {"1", "3"},
		},
		ssaapi.WithLanguage(ssaconfig.PYTHON))
}

func TestPython_DictComprehensionFlow(t *testing.T) {
	code := `
def sink(value):
    pass

values = {item: item + 1 for item in [1, 2]}
for key, value in values.items():
    sink(key)
    sink(value)
`
	ssatest.CheckSyntaxFlowContain(t, code,
		"sink(* as $param)",
		map[string][]string{
			"param": {"1", "2", "3"},
		},
		ssaapi.WithLanguage(ssaconfig.PYTHON))
}

func TestPython_ForTupleDestructuringFlow(t *testing.T) {
	code := `
def sink(value):
    pass

for left, right in [(1, 2), (3, 4)]:
    sink(left)
    sink(right)
`
	ssatest.CheckSyntaxFlowContain(t, code,
		"sink(* as $param)",
		map[string][]string{
			"param": {"1", "2", "3", "4"},
		},
		ssaapi.WithLanguage(ssaconfig.PYTHON))
}

func TestPython_ForDynamicStarDestructuringFlow(t *testing.T) {
	code := `
def sink(value):
    pass

def handle(items):
    for head, *rest in items:
        sink(head)
        sink(rest)

handle([(1, 2, 3, 4)])
`
	ssatest.CheckSyntaxFlowContain(t, code,
		"sink(* as $param)",
		map[string][]string{
			"param": {"make(any)"},
		},
		ssaapi.WithLanguage(ssaconfig.PYTHON))
}

func TestPython_TryElseFlow(t *testing.T) {
	code := `
def sink(value):
    pass

try:
    sink("body")
except:
    sink("except")
else:
    sink("else")
finally:
    sink("finally")
`
	ssatest.CheckSyntaxFlowContain(t, code,
		"sink(* as $param)",
		map[string][]string{
			"param": {"\"body\"", "\"else\"", "\"finally\""},
		},
		ssaapi.WithLanguage(ssaconfig.PYTHON))
}

func TestPython_TryExceptNamedValueTransportFlow(t *testing.T) {
	code := `
def sink(value):
    pass

try:
    raise "boom"
except ValueError as err:
    sink(err)
`
	ssatest.CheckSyntaxFlowContain(t, code,
		"sink(* as $param)",
		map[string][]string{
			"param": {"\"boom\""},
		},
		ssaapi.WithLanguage(ssaconfig.PYTHON))
}

func TestPython_TryExceptStaticTypeSelectionFlow(t *testing.T) {
	code := `
def sink(value):
    pass

try:
    raise TypeError("boom")
except ValueError as err:
    sink("wrong")
except TypeError as err:
    sink(err)
`
	ssatest.CheckSyntaxFlowContain(t, code,
		"sink(* as $param)",
		map[string][]string{
			"param": {"TypeError(\"boom\")"},
		},
		ssaapi.WithLanguage(ssaconfig.PYTHON))
}

func TestPython_TryExceptTupleStaticTypeSelectionFlow(t *testing.T) {
	code := `
def sink(value):
    pass

try:
    raise TypeError("boom")
except (ValueError, KeyError) as err:
    sink("wrong")
except (TypeError, OverflowError) as err:
    sink(err)
`
	ssatest.CheckSyntaxFlowContain(t, code,
		"sink(* as $param)",
		map[string][]string{
			"param": {"TypeError(\"boom\")"},
		},
		ssaapi.WithLanguage(ssaconfig.PYTHON))
}

func TestPython_MatchCaseFlow(t *testing.T) {
	code := `
def sink(value):
    pass

value = 2
match value:
    case 1:
        sink("one")
    case 2 | 3:
        sink("many")
    case _:
        sink("other")
`
	ssatest.CheckSyntaxFlowContain(t, code,
		"sink(* as $param)",
		map[string][]string{
			"param": {"\"many\""},
		},
		ssaapi.WithLanguage(ssaconfig.PYTHON))
}

func TestPython_MatchCaseSequenceCaptureFlow(t *testing.T) {
	code := `
def sink(value):
    pass

match ("demo", ".", "py"):
    case filename, ".", "py":
        sink(filename)
    case _:
        sink("other")
`
	ssatest.CheckSyntaxFlow(t, code,
		"sink(* as $param)",
		map[string][]string{
			"param": {"\"demo\""},
		},
		ssaapi.WithLanguage(ssaconfig.PYTHON))
}

func TestPython_MatchCaseSequenceCaptureGuardFallbackFlow(t *testing.T) {
	code := `
def sink(value):
    pass

match ("demo-file", ".", "py"):
    case filename, ".", "py" if filename.isidentifier():
        sink(filename)
    case _:
        sink("other")
`
	ssatest.CheckSyntaxFlow(t, code,
		"sink(* as $param)",
		map[string][]string{
			"param": {"\"other\""},
		},
		ssaapi.WithLanguage(ssaconfig.PYTHON))
}

func TestPython_MatchCaseDynamicSequenceCaptureFlow(t *testing.T) {
	code := `
def sink(value):
    pass

name = "demo.py"
match name.partition("."):
    case filename, ".", "py" if filename.isidentifier():
        sink(filename)
    case _:
        sink(name)
`
	ssatest.CheckSyntaxFlow(t, code,
		"sink(* as $param)",
		map[string][]string{
			"param": {"\"demo\""},
		},
		ssaapi.WithLanguage(ssaconfig.PYTHON))
}

func TestPython_MatchCaseStaticStarPatternFlow(t *testing.T) {
	code := `
def sink(value):
    pass

match (1, 2, 3, 4):
    case head, *rest:
        sink(head)
    case _:
        sink("other")
`
	ssatest.CheckSyntaxFlowContain(t, code,
		"sink(* as $param)",
		map[string][]string{
			"param": {"1"},
		},
		ssaapi.WithLanguage(ssaconfig.PYTHON))
}

func TestPython_MatchCaseDynamicStarPatternFlow(t *testing.T) {
	code := `
def sink(value):
    pass

def handle(data):
    match data:
        case head, *rest:
            sink(head)
            sink(rest)
        case _:
            sink("other")

handle((1, 2, 3, 4))
`
	ssatest.CheckSyntaxFlowContain(t, code,
		"sink(* as $param)",
		map[string][]string{
			"param": {"ParameterMember-parameter[0].0", "make(any)"},
		},
		ssaapi.WithLanguage(ssaconfig.PYTHON))
}
