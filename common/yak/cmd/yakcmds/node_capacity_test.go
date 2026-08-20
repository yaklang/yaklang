package yakcmds

import "testing"

func TestNodeMaxRunningJobsFromInt(t *testing.T) {
	for _, tt := range []struct {
		name    string
		value   int
		want    uint32
		wantErr bool
	}{
		{name: "default", value: 1, want: 1},
		{name: "unlimited", value: 0, want: 0},
		{name: "negative", value: -1, wantErr: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := nodeMaxRunningJobsFromInt(tt.value)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %t", err, tt.wantErr)
			}
			if got != tt.want {
				t.Fatalf("max = %d, want %d", got, tt.want)
			}
		})
	}
}
