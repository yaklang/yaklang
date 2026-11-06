package jsp

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// Regression for pprof F3 (20260519): preprocessJavascriptHrefQuotes used to
// loop forever on ="javascript:..."> when the href body contains no embedded ".
const maxPreprocessJavascriptHrefQuotesDuration = 200 * time.Millisecond

func TestPreprocessJavascriptHrefQuotes_noInfiniteLoopOnPlainJavascriptHref(t *testing.T) {
	src := `<jsp:page/><a href="javascript:alert(1)">click</a>`
	require.True(t, strings.Contains(src, `="javascript:`),
		"fixture must hit preprocessJavascriptHrefQuotes marker")

	done := make(chan struct{})
	var got string
	go func() {
		got = preprocessJavascriptHrefQuotes(src)
		close(done)
	}()

	select {
	case <-done:
		// After fix: body has no inner quotes, output should be unchanged.
		require.Equal(t, src, got)
	case <-time.After(maxPreprocessJavascriptHrefQuotesDuration):
		t.Fatal("preprocessJavascriptHrefQuotes did not return: infinite loop on " +
			`="javascript:..."> when href body has no embedded double quotes`)
	}
}

func TestFront_javascriptHrefWithoutInnerQuotesCompletesQuickly(t *testing.T) {
	src := `<jsp:page/><a href="javascript:alert(1)">click</a>`

	done := make(chan error, 1)
	go func() {
		_, err := Front(src)
		done <- err
	}()

	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(maxPreprocessJavascriptHrefQuotesDuration):
		t.Fatal("jsp.Front did not return: preprocessJavascriptHrefQuotes infinite loop")
	}
}

func TestPreprocessJavascriptHrefQuotes_replacesEmbeddedQuotesInBody(t *testing.T) {
	src := `<jsp:page/><a href="javascript:foo("bar")">click</a>`
	got := preprocessJavascriptHrefQuotes(src)
	require.Equal(t, `<jsp:page/><a href="javascript:foo('bar')">click</a>`, got)
}

// A large page whose HTML cannot be structured must not enter LL full-context
// recovery. That path is what killed the 2026-10-22 coremail scan: it spent
// minutes per page and grew the heap to 20.5GB before the OOM killer fired.
// The page below is adversarial on purpose (thousands of unclosed tags); the
// bounded SLL pass still finishes in seconds, while LL did not finish at all.
func TestFront_largeMalformedPageAvoidsFullContextRecovery(t *testing.T) {
	var page strings.Builder
	page.WriteString("<%@ page contentType=\"text/html\" %>\n<html><body>\n<c:out value=\"${title}\"/>\n")
	for page.Len() < 96*1024 {
		page.WriteString(`<div id="row"><span class="x"><input type="text" value="`)
		page.WriteString(strings.Repeat("a", 40))
		page.WriteString(`" name="n"><ul><li><a href="/x?q=`)
		page.WriteString(strings.Repeat("b", 20))
	}
	page.WriteString("<div class=unterminated")

	start := time.Now()
	_, err := Front(page.String())
	elapsed := time.Since(start)
	require.Error(t, err)
	require.Less(t, elapsed, 30*time.Second, "large malformed JSP did not stay bounded, got %s", elapsed)
}

// A small page with one malformed tag still carries structure worth keeping:
// the bounded recovering pass returns a partial tree instead of discarding the
// page. Measured over 700 real coremail pages this path stays under 100ms.
func TestFront_smallMalformedPageKeepsStructure(t *testing.T) {
	page := `<div class="single-line">
  <td align="right"><fmt:message key=".info.privilege"/></td>
  <td><input type="checkbox" name="supportApproval" value="1" ${bean.supportApproval == '1' ? 'checked' : ''}>&nbsp;<fmt:message key=".info.approval_project"/></td>
</div>`
	start := time.Now()
	ast, err := Front(page)
	elapsed := time.Since(start)
	require.NoError(t, err)
	require.NotNil(t, ast)
	require.Less(t, elapsed, 2*time.Second, "recovering parse took %s", elapsed)
}
