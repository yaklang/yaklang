package main

import (
	"errors"
	"testing"
)

func TestShouldRunDistYak(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		args []string
		want bool
	}{
		{
			name: "node mode",
			args: []string{"legion-smoke-node", "-api-url", "http://127.0.0.1:8080"},
			want: false,
		},
		{
			name: "distyak mode",
			args: []string{"legion-smoke-node", "distyak", "/tmp/test.yak"},
			want: true,
		},
		{
			name: "empty args",
			args: []string{"legion-smoke-node"},
			want: false,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := shouldRunDistYak(tc.args); got != tc.want {
				t.Fatalf("shouldRunDistYak(%v) = %v, want %v", tc.args, got, tc.want)
			}
		})
	}
}

func TestInitializeAISessionCapabilitiesOnlyRunsForAISession(t *testing.T) {
	called := 0
	syncFn := func() error {
		called++
		return nil
	}

	if err := initializeAISessionCapabilities("host", syncFn); err != nil {
		t.Fatalf("host initialization returned error: %v", err)
	}
	if called != 0 {
		t.Fatalf("host node unexpectedly synchronized AI capabilities %d time(s)", called)
	}

	if err := initializeAISessionCapabilities(" ai_session ", syncFn); err != nil {
		t.Fatalf("AI session initialization returned error: %v", err)
	}
	if called != 1 {
		t.Fatalf("AI session synchronized capabilities %d time(s), want 1", called)
	}
}

func TestInitializeAISessionCapabilitiesReturnsSyncFailure(t *testing.T) {
	want := errors.New("profile database unavailable")
	err := initializeAISessionCapabilities("ai_session", func() error { return want })
	if !errors.Is(err, want) {
		t.Fatalf("initializeAISessionCapabilities() error = %v, want wrapped %v", err, want)
	}
	if err == nil || err.Error() != "initialize AI session capabilities: profile database unavailable" {
		t.Fatalf("unexpected initialization error: %v", err)
	}
}
