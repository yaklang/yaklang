package fp

import (
	"context"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/fp/fingerprint"
	"github.com/yaklang/yaklang/common/fp/fingerprint/rule"
	"github.com/yaklang/yaklang/common/schema"
)

func TestDefaultWebRulesCompileOnceAndKeepHits(t *testing.T) {
	rules, err := GetDefaultWebFingerprintRules()
	require.NoError(t, err)
	require.NotEmpty(t, rules)
	root := 0
	for _, item := range rules {
		if item != nil && item.Method == "regexp" && item.MatchParam != nil && item.MatchParam.RegexpPattern != "" &&
			(item.WebPath == "" || item.WebPath == "/") {
			root++
		}
	}
	t.Logf("web rules %d, root page regexp %d", len(rules), root)

	banner := []byte("HTTP/1.1 200 OK\r\nServer: nginx/1.24.0\r\n\r\n<html><title>nginx</title></html>")
	getter := func(string) (*rule.MatchResource, error) {
		return rule.NewHttpResource(banner), nil
	}
	names := func(cpes []*schema.CPE) []string {
		out := make([]string, 0, len(cpes))
		for _, cpe := range cpes {
			if cpe == nil {
				continue
			}
			out = append(out, cpe.Product+"@"+cpe.Version)
		}
		sort.Strings(out)
		return out
	}

	matcher := fingerprint.NewMatcher()
	ctx := context.Background()
	start := time.Now()
	first := matcher.MatchResource(ctx, 8, rules, getter)
	cold := time.Since(start)
	const rounds = 50
	start = time.Now()
	var second []*schema.CPE
	for i := 0; i < rounds; i++ {
		second = matcher.MatchResource(ctx, 8, rules, getter)
		require.Equal(t, names(first), names(second))
	}
	hot := time.Since(start) / rounds
	t.Logf("cold %s hot avg %s over %d cpes %d", cold, hot, rounds, len(second))
	require.Less(t, hot, 5*time.Millisecond)

	start = time.Now()
	var sequential []string
	for _, item := range rules {
		cpe, err := rule.Execute(getter, item)
		if err != nil || cpe == nil {
			continue
		}
		sequential = append(sequential, cpe.Product+"@"+cpe.Version)
	}
	sort.Strings(sequential)
	sequentialOnce := time.Since(start)
	start = time.Now()
	for i := 0; i < rounds; i++ {
		for _, item := range rules {
			_, _ = rule.Execute(getter, item)
		}
	}
	sequentialHot := time.Since(start) / rounds
	t.Logf("sequential first %s hot avg %s hits %d", sequentialOnce, sequentialHot, len(sequential))
	require.Equal(t, sequential, names(first))

	fat := make([]byte, 20480)
	copy(fat, banner)
	for i := len(banner); i < len(fat); i++ {
		fat[i] = 'a'
	}
	fatGetter := func(string) (*rule.MatchResource, error) {
		return rule.NewHttpResource(fat), nil
	}
	start = time.Now()
	fatFirst := matcher.MatchResource(ctx, 8, rules, fatGetter)
	for i := 0; i < rounds; i++ {
		got := matcher.MatchResource(ctx, 8, rules, fatGetter)
		require.Equal(t, names(fatFirst), names(got))
	}
	fatHot := time.Since(start) / (rounds + 1)
	start = time.Now()
	for i := 0; i < rounds; i++ {
		for _, item := range rules {
			_, _ = rule.Execute(fatGetter, item)
		}
	}
	fatSequential := time.Since(start) / rounds
	t.Logf("20480-byte page prefilter avg %s sequential avg %s cpes %d", fatHot, fatSequential, len(fatFirst))
	require.Less(t, fatHot, fatSequential)
}
