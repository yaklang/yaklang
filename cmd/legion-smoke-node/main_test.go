package main

import (
	"errors"
	"testing"
)

func TestHostDockerEndpoint(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		kind     string
		endpoint string
		want     string
	}{
		{name: "host", kind: "host", endpoint: " tcp://runtime-host:2376 ", want: "tcp://runtime-host:2376"},
		{name: "default host kind", endpoint: "unix:///var/run/docker.sock", want: "unix:///var/run/docker.sock"},
		{name: "AI session cannot advertise host daemon", kind: " ai_session ", endpoint: "tcp://runtime-host:2376", want: ""},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := hostDockerEndpoint(tt.kind, tt.endpoint); got != tt.want {
				t.Fatalf("hostDockerEndpoint(%q, %q) = %q, want %q", tt.kind, tt.endpoint, got, tt.want)
			}
		})
	}
}

func TestHostEngineValue(t *testing.T) {
	t.Parallel()
	if got := hostEngineValue("host", " sha256-e2 "); got != "sha256-e2" {
		t.Fatalf("host engine value = %q", got)
	}
	if got := hostEngineValue("ai_session", "sha256-e2"); got != "" {
		t.Fatalf("AI session leaked host engine identity = %q", got)
	}
}

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
