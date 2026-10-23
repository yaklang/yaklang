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

func TestForTokensLiteralAndMixedELParse(t *testing.T) {
	src := `<c:set var="queryIntOptions">
<option value="<"><fmt:message key="op_int_lt" /></option>
</c:set>
<%--StripedByPackage--%>
<c:forTokens items="0500|0930|1200" delims="|" var="boundary">
  <c:set var="greetKey" value="greet_${boundary}"/>
</c:forTokens>
<c:set var="to" value="${to}${empty cc ? '' : ','}${cc}"/>
<c:forTokens items="contains excludes = != startsWith endsWith" delims=" " var="key">
  <c:out value="${key}"/>
</c:forTokens>`
	code := convertJSPForTest(t, src)
	if strings.Contains(code, "--Striped") || strings.Contains(code, "<%") {
		t.Fatalf("jsp comment leaked into Java:\n%s", code)
	}
	if !strings.Contains(code, `String.valueOf("0500|0930|1200")`) {
		t.Fatalf("literal forTokens items must be a Java string:\n%s", code)
	}
	if !strings.Contains(code, `"greet_" + elExpr.parse("boundary")`) {
		t.Fatalf("mixed literal/EL must be joined with +:\n%s", code)
	}
	if !strings.Contains(code, `elExpr.parse("to") + elExpr.parse("empty cc ? '' : ','") + elExpr.parse("cc")`) {
		t.Fatalf("adjacent EL must be joined with +:\n%s", code)
	}
	if !strings.Contains(code, `String.valueOf("contains excludes = != startsWith endsWith")`) {
		t.Fatalf("word-list items must be a Java string:\n%s", code)
	}
}

// coremail writes items as a nested jsp:attribute, not an XML attribute.
func TestForTokensItemsFromJSPAttribute(t *testing.T) {
	src := `<c:forEach begin="1" end="2" var="sloganIndex">
  <c:forTokens delims="|" var="slogan">
    <jsp:attribute name="items">
      <fmt:message key="login_slogan${sloganIndex}" />
    </jsp:attribute>
    <jsp:body>
      <div>${slogan}</div>
    </jsp:body>
  </c:forTokens>
</c:forEach>`
	code := convertJSPForTest(t, src)
	if strings.Contains(code, "jsp:attribute") || strings.Contains(code, "jsp:body") {
		t.Fatalf("jsp:attribute wrapper leaked into Java:\n%s", code)
	}
	if !strings.Contains(code, `String.valueOf("login_slogan" + elExpr.parse("sloganIndex"))`) {
		t.Fatalf("items from jsp:attribute missing:\n%s", code)
	}
	if !strings.Contains(code, `elExpr.parse("slogan")`) {
		t.Fatalf("jsp:body was dropped:\n%s", code)
	}
}

// The items form must keep working exactly as before.
func TestForeachItemsStillIteratesEl(t *testing.T) {
	code := convertJSPForTest(t, `<c:forEach items="${list}" var="x">${x}</c:forEach>`)
	if !strings.Contains(code, `for (Object x : elExpr.parse("list")) {`) {
		t.Fatalf("items loop regressed:\n%s", code)
	}
}
