package jsp

import (
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/utils/memedit"
)

func convertJSPForTest(t *testing.T, src string) string {
	t.Helper()
	ast, err := Front(src)
	if err != nil {
		t.Fatalf("parse JSP: %v", err)
	}
	visitor := NewJSPVisitor()
	visitor.Editor = memedit.NewMemEditor(src)
	if err := visitJspDocumentsSafely(visitor, ast); err != nil {
		t.Fatalf("visit JSP: %v", err)
	}
	var code strings.Builder
	for _, instruction := range visitor.GetInstructions() {
		code.WriteString(instruction.Text)
		code.WriteByte('\n')
	}
	return code.String()
}

// JSTL forEach has a counting form that uses begin/end instead of items.
// coremail ships 76 of them; the converter used to log "must have items" and
// drop the whole body, so the generated Java lost every statement in the loop.
func TestForeachRangeKeepsBodyAndBuildsCountingLoop(t *testing.T) {
	code := convertJSPForTest(t, `<c:forEach begin="1" end="2" var="sloganIndex"><p>${sloganIndex}</p></c:forEach>`)
	if !strings.Contains(code, "for (int sloganIndex = 1; sloganIndex <= 2; sloganIndex += 1) {") {
		t.Fatalf("counting loop missing:\n%s", code)
	}
	if !strings.Contains(code, `elExpr.parse("sloganIndex")`) {
		t.Fatalf("loop body dropped:\n%s", code)
	}
}

func TestForeachRangeAcceptsELBoundsAndStep(t *testing.T) {
	code := convertJSPForTest(t, `<c:forEach begin="${0}" end="${count - 1}" step="2" var="key"><b>${key}</b></c:forEach>`)
	if !strings.Contains(code, `for (int key = ((Number)elExpr.parse("0")).intValue(); key <= ((Number)elExpr.parse("count - 1")).intValue(); key += 2) {`) {
		t.Fatalf("EL bounds must become int expressions:\n%s", code)
	}
	if !strings.Contains(code, `elExpr.parse("key")`) {
		t.Fatalf("loop body dropped:\n%s", code)
	}
}

func TestForeachRangeWithoutVarUsesSyntheticIndex(t *testing.T) {
	code := convertJSPForTest(t, `<c:forEach begin="0" end="3"><i>static</i></c:forEach>`)
	if !strings.Contains(code, "for (int _jspForEachIndex = 0; _jspForEachIndex <= 3; _jspForEachIndex += 1) {") {
		t.Fatalf("numeric loop without var must still compile:\n%s", code)
	}
}

// The items form must keep working exactly as before.
func TestForeachItemsStillIteratesEl(t *testing.T) {
	code := convertJSPForTest(t, `<c:forEach items="${list}" var="x">${x}</c:forEach>`)
	if !strings.Contains(code, `for (Object x : elExpr.parse("list")) {`) {
		t.Fatalf("items loop regressed:\n%s", code)
	}
}
