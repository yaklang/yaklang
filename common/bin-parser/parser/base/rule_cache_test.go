package base

import (
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	yaml "github.com/yaklang/yaklang/common/utils/orderedyaml"
)

func TestParseRuleCacheReturnsIsolatedTrees(t *testing.T) {
	const rule = "application-layer/ntp.yaml"

	first, err := ParseRule(rule)
	require.NoError(t, err)
	second, err := ParseRule(rule)
	require.NoError(t, err)

	require.NotSame(t, first, second)
	require.NotSame(t, first.Cfg, second.Cfg)
	require.NotSame(t, first.Ctx, second.Ctx)
	require.NotSame(t, first.Children[0], second.Children[0])

	first.Cfg.SetItem("cache-isolation", true)
	first.Ctx.SetItem("cache-isolation", true)
	first.Children[0].Cfg.SetItem("cache-isolation", true)
	first.Children = nil

	require.False(t, second.Cfg.Has("cache-isolation"))
	require.False(t, second.Ctx.Has("cache-isolation"))
	require.False(t, second.Children[0].Cfg.Has("cache-isolation"))
	require.NotEmpty(t, second.Children)

	third, err := ParseRule(rule)
	require.NoError(t, err)
	require.False(t, third.Cfg.Has("cache-isolation"))
	require.False(t, third.Ctx.Has("cache-isolation"))
	require.NotEmpty(t, third.Children)
}

func TestParseRuleCacheConcurrentIsolation(t *testing.T) {
	const (
		rule    = "application-layer/ntp.yaml"
		workers = 32
	)

	var wg sync.WaitGroup
	errs := make(chan error, workers)
	roots := make(chan *Node, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			root, err := ParseRule(rule)
			if err != nil {
				errs <- err
				return
			}
			origin := root.Origin.(yaml.MapSlice)
			if origin[0].Key != "endian" || origin[0].Value != "big" {
				errs <- fmt.Errorf("worker %d received another tree's YAML mutation: %v", id, origin[0])
				return
			}
			origin[0].Value = id
			root.Ctx.SetItem("worker", id)
			if got := root.Ctx.GetItem("worker"); got != id {
				errs <- fmt.Errorf("worker context leaked: got %v, want %d", got, id)
				return
			}
			roots <- root
		}(i)
	}
	wg.Wait()
	close(errs)
	close(roots)

	for err := range errs {
		require.NoError(t, err)
	}
	seenRoots := make(map[*Node]struct{}, workers)
	seenContexts := make(map[*NodeContext]struct{}, workers)
	for root := range roots {
		seenRoots[root] = struct{}{}
		seenContexts[root.Ctx] = struct{}{}
	}
	require.Len(t, seenRoots, workers)
	require.Len(t, seenContexts, workers)
}

func TestParseRuleCacheIsolatesOriginAndNestedConfig(t *testing.T) {
	const source = `
metadata:
  tags:
    - original
    - label: original
Package:
  Message:
    Value: uint8
`
	var document yaml.MapSlice
	require.NoError(t, yaml.Unmarshal([]byte(source), &document))
	// Exercise mutable YAML config values as well as Node.Origin. Embedded
	// rules use the same decoded types, but most current configs are scalars.
	rule := "cache-test/" + t.Name() + ".yaml"
	ruleDocumentCache.Store(rule, document)
	t.Cleanup(func() { ruleDocumentCache.Delete(rule) })

	first, err := ParseRule(rule)
	require.NoError(t, err)
	second, err := ParseRule(rule)
	require.NoError(t, err)

	first.Origin.(yaml.MapSlice)[0].Key = "changedMetadata"
	first.Children[0].Children[0].Origin.(yaml.MapSlice)[0].Value = "uint16"
	metadata := first.Cfg.GetItem("metadata").(yaml.MapSlice)
	tags := metadata[0].Value.([]any)
	tags[0] = "changed"
	tags[1].(yaml.MapSlice)[0].Value = "changed"

	assertOriginal := func(root *Node) {
		t.Helper()
		require.Equal(t, "metadata", root.Origin.(yaml.MapSlice)[0].Key)
		require.Equal(t, "uint8", root.Children[0].Children[0].Origin.(yaml.MapSlice)[0].Value)
		metadata := root.Cfg.GetItem("metadata").(yaml.MapSlice)
		tags := metadata[0].Value.([]any)
		require.Equal(t, "original", tags[0])
		require.Equal(t, "original", tags[1].(yaml.MapSlice)[0].Value)
	}
	assertOriginal(second)
	third, err := ParseRule(rule)
	require.NoError(t, err)
	assertOriginal(third)
}
