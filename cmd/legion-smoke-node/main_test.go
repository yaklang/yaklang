package main

import "testing"

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
