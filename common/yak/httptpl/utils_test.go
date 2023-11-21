package httptpl

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/httptpl/utils"
)

func TestExtractorVarsFromUrl(t *testing.T) {
	tcs := []struct {
		input string
		want  map[string]string
	}{
		{
			input: "http://127.0.0.1",
			want: map[string]string{
				"Host":     "127.0.0.1",
				"Port":     "80",
				"Hostname": "127.0.0.1",
				"RootURL":  "http://127.0.0.1",
				"BaseURL":  "http://127.0.0.1",
				"Path":     "/",
				"File":     "",
				"Schema":   "http",
			},
		},
		{
			input: "http://127.0.0.1/file.txt",
			want: map[string]string{
				"Host":     "127.0.0.1",
				"Port":     "80",
				"Hostname": "127.0.0.1",
				"RootURL":  "http://127.0.0.1",
				"BaseURL":  "http://127.0.0.1/file.txt",
				"Path":     "/file.txt",
				"File":     "file.txt",
				"Schema":   "http",
			},
		},
		{
			input: "https://127.0.0.1/file.txt",
			want: map[string]string{
				"Host":     "127.0.0.1",
				"Port":     "443",
				"Hostname": "127.0.0.1",
				"RootURL":  "https://127.0.0.1",
				"BaseURL":  "https://127.0.0.1/file.txt",
				"Path":     "/file.txt",
				"File":     "file.txt",
				"Schema":   "https",
			},
		},
		{
			input: "http://127.0.0.1:8888/file.txt",
			want: map[string]string{
				"Host":     "127.0.0.1",
				"Port":     "8888",
				"Hostname": "127.0.0.1:8888",
				"RootURL":  "http://127.0.0.1:8888",
				"BaseURL":  "http://127.0.0.1:8888/file.txt",
				"Path":     "/file.txt",
				"File":     "file.txt",
				"Schema":   "http",
			},
		},
		{
			input: "https://127.0.0.1:8443/file.txt",
			want: map[string]string{
				"Host":     "127.0.0.1",
				"Port":     "8443",
				"Hostname": "127.0.0.1:8443",
				"RootURL":  "https://127.0.0.1:8443",
				"BaseURL":  "https://127.0.0.1:8443/file.txt",
				"Path":     "/file.txt",
				"File":     "file.txt",
				"Schema":   "https",
			},
		},
		{
			input: "https://127.0.0.1:8443/subpath/file.txt",
			want: map[string]string{
				"Host":     "127.0.0.1",
				"Port":     "8443",
				"Hostname": "127.0.0.1:8443",
				"RootURL":  "https://127.0.0.1:8443",
				"BaseURL":  "https://127.0.0.1:8443/subpath/file.txt",
				"Path":     "/subpath/file.txt",
				"File":     "file.txt",
				"Schema":   "https",
			},
		},
	}

	for _, tc := range tcs {
		gotMap := utils.ExtractorVarsFromUrl(tc.input)
		for varName, got := range gotMap {
			want, ok := tc.want[varName]
			if !ok {
				t.Errorf("[%s] %s not found", tc.input, varName)
			}
			if want != got {
				t.Errorf("[%s] var[%s] %s(got) != %s(want)", tc.input, varName, want, got)
			}
		}
	}
}
