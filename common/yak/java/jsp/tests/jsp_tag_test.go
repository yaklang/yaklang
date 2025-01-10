package tests

import (
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/java/jsp"
	"testing"
)

func TestJSP_JSTL_TAG(t *testing.T) {
	tests := []struct {
		name string
		code string
	}{
		{name: "pure html", code: "<html><body><h1>Hello World</h1></body></html>"},
		{name: "core out", code: "<c:out value='${name}'/>"},
		{name: "pure code", code: "<% out.println(\"Hello World\"); %>"},
		{
			name: "jsp script in html attribute",
			code: `<script type="text/javascript" src="<%=request.getContextPath() %>/proRes/js/design/draw/js/util/AssigeenTypeConfig.js?t=<%=Math.random()%>" charset="UTF-8"></script>`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := jsp.Front(tt.code)
			require.NoError(t, err)
		})
	}
}
