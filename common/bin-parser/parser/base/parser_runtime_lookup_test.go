package base

import (
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParserRuntimeLookupConcurrentNamesAndReplacement(t *testing.T) {
	r := newParserRuntimeMap()
	var created atomic.Int32
	a := &parserFactoryRegistration{factory: func() Parser { return &parserFactoryTestParser{id: int(created.Add(1))} }}
	b := &parserFactoryRegistration{factory: func() Parser { return &parserFactoryTestParser{id: int(created.Add(1))} }}
	names := []string{"primary", "other", "alias"}
	registrations := []*parserFactoryRegistration{a, b, a}
	expected := make([]Parser, len(names))
	for i, name := range names {
		var err error
		expected[i], err = r.parser(name, registrations[i])
		require.NoError(t, err)
	}
	require.NotSame(t, expected[0], expected[2], "different names retain their own factory instance")
	primary := r.cached.Load()
	var wg sync.WaitGroup
	for worker := 0; worker < 4; worker++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			for i := 0; i < 500; i++ {
				index := (worker + i) % len(names)
				name, registration := names[index], registrations[index]
				p, err := r.parser(name, registration)
				if err != nil || p != expected[index] || r.load(name, registration) != p {
					t.Errorf("wrong runtime for %s: %v", name, err)
				}
				r.bind(name, registration, p)
			}
		}(worker)
	}
	wg.Wait()
	require.Same(t, primary, r.cached.Load(), "alternating names should not churn the cache")
	require.EqualValues(t, 3, created.Load())
	replacement := &parserFactoryRegistration{factory: func() Parser { return &parserFactoryTestParser{id: 99} }}
	require.Nil(t, r.load("primary", replacement))
	p, err := r.parser("primary", replacement)
	require.NoError(t, err)
	require.Equal(t, 99, p.(*parserFactoryTestParser).id)
	require.Same(t, p, r.load("primary", replacement))
	require.Nil(t, r.load("primary", a))
	require.Same(t, expected[2], r.load("alias", a))
	broken := &parserFactoryRegistration{}
	_, err = r.parser("primary", broken)
	require.ErrorContains(t, err, "nil factory")
	require.Nil(t, r.load("primary", broken))
	require.Same(t, p, r.load("primary", replacement))
}
