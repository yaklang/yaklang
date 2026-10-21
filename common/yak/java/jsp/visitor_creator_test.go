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
	require.Less(t, elapsed, 3*time.Second, "large malformed JSP entered full-context recovery, got %s", elapsed)
}
