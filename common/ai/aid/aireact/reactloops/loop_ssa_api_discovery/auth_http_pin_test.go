package loop_ssa_api_discovery

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
)

func TestPinHTTPParamsToProbeTarget_CorrectsWrongURL(t *testing.T) {
	target := HttpProbeTarget{
		VerifiedHttpApiID: 53,
		Method:            "POST",
		PathPattern:       "/admin/cmsUserSurvey/save",
		FullSampleURL:     "http://192.168.1.4:8080/admin/cmsUserSurvey/save",
	}
	params := aitool.InvokeParams{
		"method": "POST",
		"url":    "http://192.168.1.4:8080/admin/cmsSite/update.html",
	}
	out, notes := pinHTTPParamsToProbeTarget(params, &target)
	require.Contains(t, strings.Join(notes, " "), "corrected url")
	require.Equal(t, target.FullSampleURL, out["url"])
}

func TestPinHTTPParamsToProbeTarget_CorrectsMethod(t *testing.T) {
	target := HttpProbeTarget{
		VerifiedHttpApiID: 50,
		Method:            "POST",
		FullSampleURL:     "http://192.168.1.4:8080/admin/sysUser/save",
	}
	params := aitool.InvokeParams{
		"method": "GET",
		"url":    target.FullSampleURL,
	}
	out, notes := pinHTTPParamsToProbeTarget(params, &target)
	require.Contains(t, strings.Join(notes, " "), "corrected method")
	require.Equal(t, "POST", out["method"])
}

func TestMergeResponseCookiesIntoCredential(t *testing.T) {
	cred := &store.AuthCredential{
		HeadersJSON: `{"Cookie":"JSESSIONID=old; PUBLICCMS_ADMIN=1_oldtoken"}`,
	}
	content := "HTTP/1.1 404\r\nSet-Cookie: JSESSIONID=newsession; Path=/\r\n\r\n"
	updated, notes := mergeResponseCookiesIntoCredential(cred, content)
	require.True(t, updated)
	require.NotEmpty(t, notes)
	require.Contains(t, cred.HeadersJSON, "JSESSIONID=newsession")
	require.Contains(t, cred.HeadersJSON, "PUBLICCMS_ADMIN=1_oldtoken")
}
