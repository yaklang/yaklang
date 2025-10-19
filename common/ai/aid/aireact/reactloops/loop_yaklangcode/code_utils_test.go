package loop_yaklangcode

import (
	"testing"
)

func TestPrettifyAITagYaklangCode_NormalCaseWithLineNumbers(t *testing.T) {
	// 测试正常情况：连续的行号
	input := `17 | hello = "a"
18 | b = 2
19 | if a > 1 { ... }`
	start, end, result, fixed := prettifyAITagCode(input)
	if !fixed {
		t.Fatalf("expected fixed=true, got fixed=%v", fixed)
	}
	if start != 17 {
		t.Fatalf("expected start=17, got start=%d", start)
	}
	if end != 19 {
		t.Fatalf("expected end=19, got end=%d", end)
	}
	expected := `hello = "a"
b = 2
if a > 1 { ... }`
	if result != expected {
		t.Fatalf("expected result=%q, got result=%q", expected, result)
	}
}

func TestPrettifyAITagYaklangCode_WithLeadingEmptyLines(t *testing.T) {
	// 测试前面有空行
	input := `
17 | hello = "a"
18 | b = 2`
	start, end, result, fixed := prettifyAITagCode(input)
	if !fixed {
		t.Fatalf("expected fixed=true, got fixed=%v", fixed)
	}
	if start != 17 {
		t.Fatalf("expected start=17, got start=%d", start)
	}
	if end != 18 {
		t.Fatalf("expected end=18, got end=%d", end)
	}
	expected := `hello = "a"
b = 2`
	if result != expected {
		t.Fatalf("expected result=%q, got result=%q", expected, result)
	}
}

func TestPrettifyAITagYaklangCode_WithTrailingEmptyLines(t *testing.T) {
	// 测试后面有空行
	input := `17 | hello = "a"
18 | b = 2

`
	start, end, result, fixed := prettifyAITagCode(input)
	if !fixed {
		t.Fatalf("expected fixed=true, got fixed=%v", fixed)
	}
	if start != 17 {
		t.Fatalf("expected start=17, got start=%d", start)
	}
	if end != 18 {
		t.Fatalf("expected end=18, got end=%d", end)
	}
	expected := `hello = "a"
b = 2`
	if result != expected {
		t.Fatalf("expected result=%q, got result=%q", expected, result)
	}
}

func TestPrettifyAITagYaklangCode_WithEmptyLinesInMiddle(t *testing.T) {
	// 测试中间有空行，但行号连续
	input := `17 | hello = "a"

18 | if a > 1 { ... }`
	start, end, result, fixed := prettifyAITagCode(input)
	if !fixed {
		t.Fatalf("expected fixed=true, got fixed=%v", fixed)
	}
	if start != 17 {
		t.Fatalf("expected start=17, got start=%d", start)
	}
	if end != 18 {
		t.Fatalf("expected end=18, got end=%d", end)
	}
	expected := `hello = "a"

if a > 1 { ... }`
	if result != expected {
		t.Fatalf("expected result=%q, got result=%q", expected, result)
	}
}

func TestPrettifyAITagYaklangCode_NoLineNumbers(t *testing.T) {
	// 测试没有行号的情况
	input := `hello = "a"
b = 2
if a > 1 { ... }`
	start, end, result, fixed := prettifyAITagCode(input)
	if fixed {
		t.Fatalf("expected fixed=false, got fixed=%v", fixed)
	}
	if start != 0 {
		t.Fatalf("expected start=0, got start=%d", start)
	}
	if end != 0 {
		t.Fatalf("expected end=0, got end=%d", end)
	}
	if result != input {
		t.Fatalf("expected result to be unchanged")
	}
}

func TestPrettifyAITagYaklangCode_DiscontiguousLineNumbers(t *testing.T) {
	// 测试行号不连续的情况
	input := `17 | hello = "a"
19 | b = 2
20 | if a > 1 { ... }`
	start, end, result, fixed := prettifyAITagCode(input)
	if fixed {
		t.Fatalf("expected fixed=false (discontiguous), got fixed=%v", fixed)
	}
	if start != 0 || end != 0 {
		t.Fatalf("expected start=0 and end=0 when not fixed")
	}
	if result != input {
		t.Fatalf("expected result to be unchanged when line numbers are discontiguous")
	}
}

func TestPrettifyAITagYaklangCode_MissingLineNumberInMiddle(t *testing.T) {
	// 测试中间某一行缺少行号
	input := `17 | hello = "a"
b = 2
19 | if a > 1 { ... }`
	start, end, result, fixed := prettifyAITagCode(input)
	if fixed {
		t.Fatalf("expected fixed=false (missing line number), got fixed=%v", fixed)
	}
	if start != 0 || end != 0 {
		t.Fatalf("expected start=0 and end=0 when not fixed")
	}
	if result != input {
		t.Fatalf("expected result to be unchanged when line number is missing")
	}
}

func TestPrettifyAITagYaklangCode_SingleLine(t *testing.T) {
	// 测试单行情况
	input := `42 | x = 10`
	start, end, result, fixed := prettifyAITagCode(input)
	if !fixed {
		t.Fatalf("expected fixed=true, got fixed=%v", fixed)
	}
	if start != 42 {
		t.Fatalf("expected start=42, got start=%d", start)
	}
	if end != 42 {
		t.Fatalf("expected end=42, got end=%d", end)
	}
	expected := `x = 10`
	if result != expected {
		t.Fatalf("expected result=%q, got result=%q", expected, result)
	}
}

func TestPrettifyAITagYaklangCode_EmptyInput(t *testing.T) {
	// 测试空输入
	input := ``
	_, _, result, fixed := prettifyAITagCode(input)
	if fixed {
		t.Fatalf("expected fixed=false, got fixed=%v", fixed)
	}
	if result != input {
		t.Fatalf("expected result to be empty string")
	}
}

func TestPrettifyAITagYaklangCode_OnlyEmptyLines(t *testing.T) {
	// 测试只有空行的情况
	input := `

`
	_, _, result, fixed := prettifyAITagCode(input)
	if fixed {
		t.Fatalf("expected fixed=false, got fixed=%v", fixed)
	}
	if result != input {
		t.Fatalf("expected result to be unchanged")
	}
}

func TestPrettifyAITagYaklangCode_LineNumberWithMultipleSpaces(t *testing.T) {
	// 测试行号前后有多个空格的情况
	input := `17  |  hello = "a"
18  |  b = 2`
	start, end, result, fixed := prettifyAITagCode(input)
	if !fixed {
		t.Fatalf("expected fixed=true, got fixed=%v", fixed)
	}
	if start != 17 {
		t.Fatalf("expected start=17, got start=%d", start)
	}
	if end != 18 {
		t.Fatalf("expected end=18, got end=%d", end)
	}
	expected := ` hello = "a"
 b = 2`
	if result != expected {
		t.Fatalf("expected result=%q, got result=%q", expected, result)
	}
}

func TestPrettifyAITagYaklangCode_LineNumberWithTabSpace(t *testing.T) {
	// 测试行号和代码之间的空格有特定的格式要求
	// 根据正则表达式 ^(\d+)\s+\|\s，需要至少一个空格在|后
	input := `17 | hello = "a"
18 | b = 2`
	_, _, result, fixed := prettifyAITagCode(input)
	if !fixed {
		t.Fatalf("expected fixed=true, got fixed=%v", fixed)
	}
	expected := `hello = "a"
b = 2`
	if result != expected {
		t.Fatalf("expected result=%q, got result=%q", expected, result)
	}
}

func TestPrettifyAITagYaklangCode_CodeWithSpecialCharacters(t *testing.T) {
	// 测试代码包含特殊字符
	input := `1 | func() { return "hello|world" }
2 | x := 100`
	_, _, result, fixed := prettifyAITagCode(input)
	if !fixed {
		t.Fatalf("expected fixed=true, got fixed=%v", fixed)
	}
	expected := `func() { return "hello|world" }
x := 100`
	if result != expected {
		t.Fatalf("expected result=%q, got result=%q", expected, result)
	}
}

func TestPrettifyAITagYaklangCode_CodeWithLeadingSpaces(t *testing.T) {
	// 测试代码本身包含前导空格
	input := `1 |     x := 10
2 |   y := 20`
	_, _, result, fixed := prettifyAITagCode(input)
	if !fixed {
		t.Fatalf("expected fixed=true, got fixed=%v", fixed)
	}
	expected := `    x := 10
  y := 20`
	if result != expected {
		t.Fatalf("expected result=%q, got result=%q", expected, result)
	}
}

func TestPrettifyAITagYaklangCode_LineNumberNotAtStart(t *testing.T) {
	// 测试行号不在行首的情况
	input := `some text 17 | hello = "a"
18 | b = 2`
	start, end, result, fixed := prettifyAITagCode(input)
	if fixed {
		t.Fatalf("expected fixed=false (line number not at start), got fixed=%v", fixed)
	}
	if start != 0 || end != 0 {
		t.Fatalf("expected start=0 and end=0 when not fixed")
	}
	if result != input {
		t.Fatalf("expected result to be unchanged")
	}
}

func TestPrettifyAITagYaklangCode_MissingPipeSymbol(t *testing.T) {
	// 测试缺少管道符号
	input := `17  hello = "a"
18  b = 2`
	start, end, result, fixed := prettifyAITagCode(input)
	if fixed {
		t.Fatalf("expected fixed=false (missing pipe), got fixed=%v", fixed)
	}
	if start != 0 || end != 0 {
		t.Fatalf("expected start=0 and end=0 when not fixed")
	}
	if result != input {
		t.Fatalf("expected result to be unchanged")
	}
}

func TestPrettifyAITagYaklangCode_NoSpaceAfterPipe(t *testing.T) {
	// 测试管道符后面没有空格的情况
	input := `17 |hello = "a"
18 |b = 2`
	start, end, result, fixed := prettifyAITagCode(input)
	if fixed {
		t.Fatalf("expected fixed=false (no space after pipe), got fixed=%v", fixed)
	}
	if start != 0 || end != 0 {
		t.Fatalf("expected start=0 and end=0 when not fixed")
	}
	if result != input {
		t.Fatalf("expected result to be unchanged")
	}
}

func TestPrettifyAITagYaklangCode_LargeLineNumbers(t *testing.T) {
	// 测试大的行号
	input := `999 | hello = "a"
1000 | b = 2
1001 | c = 3`
	start, end, result, fixed := prettifyAITagCode(input)
	if !fixed {
		t.Fatalf("expected fixed=true, got fixed=%v", fixed)
	}
	if start != 999 {
		t.Fatalf("expected start=999, got start=%d", start)
	}
	if end != 1001 {
		t.Fatalf("expected end=1001, got end=%d", end)
	}
	expected := `hello = "a"
b = 2
c = 3`
	if result != expected {
		t.Fatalf("expected result=%q, got result=%q", expected, result)
	}
}

func TestPrettifyAITagYaklangCode_LineNumberZero(t *testing.T) {
	// 测试行号为 0 的情况
	input := `0 | hello = "a"
1 | b = 2`
	start, end, result, fixed := prettifyAITagCode(input)
	if !fixed {
		t.Fatalf("expected fixed=true, got fixed=%v", fixed)
	}
	if start != 0 {
		t.Fatalf("expected start=0, got start=%d", start)
	}
	if end != 1 {
		t.Fatalf("expected end=1, got end=%d", end)
	}
	expected := `hello = "a"
b = 2`
	if result != expected {
		t.Fatalf("expected result=%q, got result=%q", expected, result)
	}
}

func TestPrettifyAITagYaklangCode_CodeEmpty(t *testing.T) {
	// 测试行号后面没有代码的情况
	input := `17 | 
18 | hello`
	start, end, result, fixed := prettifyAITagCode(input)
	if !fixed {
		t.Fatalf("expected fixed=true, got fixed=%v", fixed)
	}
	if start != 17 {
		t.Fatalf("expected start=17, got start=%d", start)
	}
	if end != 18 {
		t.Fatalf("expected end=18, got end=%d", end)
	}
	expected := `
hello`
	if result != expected {
		t.Fatalf("expected result=%q, got result=%q", expected, result)
	}
}

func TestPrettifyAITagYaklangCode_MultipleConsecutiveLineNumbers(t *testing.T) {
	// 测试完整的、较长的连续行号
	input := `1 | line1
2 | line2
3 | line3
4 | line4
5 | line5`
	start, end, result, fixed := prettifyAITagCode(input)
	if !fixed {
		t.Fatalf("expected fixed=true, got fixed=%v", fixed)
	}
	if start != 1 {
		t.Fatalf("expected start=1, got start=%d", start)
	}
	if end != 5 {
		t.Fatalf("expected end=5, got end=%d", end)
	}
	expected := `line1
line2
line3
line4
line5`
	if result != expected {
		t.Fatalf("expected result=%q, got result=%q", expected, result)
	}
}

func TestPrettifyAITagYaklangCode_AllLinesEmpty(t *testing.T) {
	// 测试所有行都是空行的情况
	input := `

`
	_, _, result, fixed := prettifyAITagCode(input)
	if fixed {
		t.Fatalf("expected fixed=false, got fixed=%v", fixed)
	}
	if result != input {
		t.Fatalf("expected result to be unchanged")
	}
}

func TestPrettifyAITagYaklangCode_WithCarriageReturn(t *testing.T) {
	// 测试包含回车符的情况
	input := "17 | hello = \"a\"\r\n18 | b = 2"
	start, end, result, fixed := prettifyAITagCode(input)
	if !fixed {
		t.Fatalf("expected fixed=true, got fixed=%v", fixed)
	}
	if start != 17 {
		t.Fatalf("expected start=17, got start=%d", start)
	}
	if end != 18 {
		t.Fatalf("expected end=18, got end=%d", end)
	}
	expected := `hello = "a"
b = 2`
	if result != expected {
		t.Fatalf("expected result=%q, got result=%q", expected, result)
	}
}
