package syntaxflow

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestCondition_URLGetCallFilterShouldKeepConstructorArgs(t *testing.T) {
	code := `
import java.io.InputStream;
import java.net.HttpURLConnection;
import java.net.URL;

public class UrlCase {
    public void test() throws Exception {
        URL u1 = new URL("http://example.com/a");
        HttpURLConnection c = (HttpURLConnection) u1.openConnection();

        URL u2 = new URL("http://example.com/b");
        InputStream s = u2.openStream();
    }
}
`

	tests := []struct {
		name string
		rule string
		want []string
	}{
		{
			name: "or_should_keep_both_constructor_args",
			rule: `URL?{<getCall>?{.openConnection() || .openStream()}}(,* as $output);`,
			want: []string{"http://example.com/a", "http://example.com/b"},
		},
		{
			name: "open_connection_should_keep_first_constructor_arg",
			rule: `URL?{<getCall>?{.openConnection()}}(,* as $output);`,
			want: []string{"http://example.com/a"},
		},
		{
			name: "open_stream_should_keep_second_constructor_arg",
			rule: `URL?{<getCall>?{.openStream()}}(,* as $output);`,
			want: []string{"http://example.com/b"},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			ssatest.CheckSyntaxFlowContain(t, code, tt.rule, map[string][]string{
				"output": tt.want,
			}, ssaapi.WithLanguage(ssaconfig.JAVA))
		})
	}
}
