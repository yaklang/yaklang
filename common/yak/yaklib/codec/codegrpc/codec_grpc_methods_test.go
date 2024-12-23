package codegrpc

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/authhack"
)

var (
	defaultCodecExecFlow = NewCodecExecFlow([]byte(""), nil)
)

func TestJwt(t *testing.T) {
	testData := `eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXUyJ9.eyJsb2dpbiI6InRlc3QiLCJpYXQiOiIxNzM0OTIyMTgxIn0.OGY2NDkyZWI3ZWQ3YmJkMjdiNmY0ODYwY2NjNTdiMGY3ZjAxMWM3YjkwMGYxNGViOTFiYzc4NzlkYWFmYTZmZA`
	defaultCodecExecFlow.Text = []byte(testData)
	// parse
	err := defaultCodecExecFlow.JwtParse()
	require.NoError(t, err)
	want := `{
    "alg": "HS256",
    "brute_secret_key_finished": false,
    "claims": {
        "iat": "1734922181",
        "login": "test"
    },
    "header": {
        "alg": "HS256",
        "typ": "JWS"
    },
    "is_valid": false,
    "raw": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXUyJ9.eyJsb2dpbiI6InRlc3QiLCJpYXQiOiIxNzM0OTIyMTgxIn0.OGY2NDkyZWI3ZWQ3YmJkMjdiNmY0ODYwY2NjNTdiMGY3ZjAxMWM3YjkwMGYxNGViOTFiYzc4NzlkYWFmYTZmZA",
    "secret_key": ""
}`
	// check result
	wantMap, gotMap := make(map[string]any), make(map[string]any)
	err = json.Unmarshal([]byte(want), &wantMap)
	require.NoError(t, err)
	err = json.Unmarshal(defaultCodecExecFlow.Text, &gotMap)
	require.NoError(t, err)
	require.Equal(t, wantMap, gotMap)

	// reverse sign
	err = defaultCodecExecFlow.JwtReverseSign()
	// check result
	require.NoError(t, err)
	wantToken, _, err := authhack.JwtParse(testData)
	require.ErrorIs(t, err, authhack.ErrKeyNotFound)
	gotToken, _, err := authhack.JwtParse(string(defaultCodecExecFlow.Text))
	require.ErrorIs(t, err, authhack.ErrKeyNotFound)

	require.Equal(t, wantToken.Header, gotToken.Header)
	require.Equal(t, wantToken.Claims, gotToken.Claims)
}
