package yaklib

import (
	"fmt"
	"strconv"
	"testing"

	"github.com/dop251/goja/parser"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"
)

func TestWalk(t *testing.T) {
	code := `
	setTimeout(function() {
	window.location.replace(` + strconv.Quote("http://baidu.com") + `);
	console.log("1111")
	}, 3000)
console.log("1111")
for (var i=0; i<5; i++)
{
	console.log("1111")
}
var a = 1
var b = 2
var a = 2 
if (a == b){
	console.log("1111")
}
	`
	res, err := parser.ParseFile(nil, "", code, 0)
	if err == nil {
		fmt.Printf("%v", res)
	}
}

func TestConsole(t *testing.T) {
	code := `console.log("Hello, World.");`
	_, _, err := _run(code)
	if err != nil {
		t.Fatal(err)
	}
}

func TestNativeCrypto_getRandomValues(t *testing.T) {
	t.Run("CryptoJSV3", func(t *testing.T) {
		code := `iv = CryptoJS.lib.WordArray.random(16); iv.toString();`
		_, value, err := _run(code, _libCryptoJSV3())
		if err != nil {
			t.Fatal(err)
		}
		t.Log(value.String())
	})
	t.Run("CryptoJSV4", func(t *testing.T) {
		code := `iv = CryptoJS.lib.WordArray.random(16); iv.toString();`
		_, value, err := _run(code, _libCryptoJSV4())
		if err != nil {
			t.Fatal(err)
		}
		t.Log(value.String())
	})
}

func TestRunWithCryptoJSV3(t *testing.T) {
	code := `CryptoJS.HmacSHA256("Message", "secret").toString();`
	_, value, err := _run(code, _libCryptoJSV3())
	if err != nil {
		t.Fatal(err)
	}
	t.Log(value.String())
}

func TestRunWithCryptoJSV4(t *testing.T) {
	check := func(opts ...jsRunOpts) {
		code := `CryptoJS.HmacSHA256("Message", "secret").toString();`
		_, value, err := _run(code, opts...)
		require.NoError(t, err)
		t.Log(value.String())
	}

	t.Run("auto", func(t *testing.T) {
		check()
	})
	t.Run("normal", func(t *testing.T) {
		check(_libCryptoJSV4())
	})
}

func TestRunWithJSRSASign(t *testing.T) {
	check := func(opts ...jsRunOpts) {
		code := `pemPublicKey = "-----BEGIN PUBLIC KEY-----\
	MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEAtnbrr63e/UbC8j7dXL4I\
	KaCAswHJrIWeY59Dcj5Og+W5Cgt7X+qrpOm7/ojpW+IdPVAYXdFPeZUEVe1p3j/X\
	7lsrIBg/iJ6lFDZb1TMTyF6LOFKQmz9ElMnZ1JQxwaKoP5CouYQ7ZJwtSIadUGKD\
	0zBy/b6yZ5KO4TIGmK7116BCp6GLU5PEYBPupGTULa6LZbqY3P4f9+ptgSjRKszJ\
	2MDmQwnhNu87eAwM3k8BEEaNBw7MviWTJp/hwr63MS6rhAzul6I/p5cDwMZf+UXW\
	14Q8PF3DXNJ1il44ihV6dW54Ynt77BC9ULmkAOrdMkXMp0830vK4bs1T3oGJlJdv\
	owIDAQAB\
	-----END PUBLIC KEY-----";
	publicKey = KEYUTIL.getKey(pemPublicKey);
	publicKey.encrypt("yaklang");
	`
		_, value, err := _run(code, opts...)
		require.NoError(t, err)
		t.Log(value.String())
	}

	t.Run("auto", func(t *testing.T) {
		check()
	})
	t.Run("normal", func(t *testing.T) {
		check(_libJSRSASign())
	})
}

func TestRunWithJSEncrypt(t *testing.T) {
	check := func(opts ...jsRunOpts) {
		code := `new JSEncrypt();`
		_, value, err := _run(code, opts...)
		if err != nil {
			t.Fatal(err)
		}
		t.Log(value.String())
	}
	t.Run("auto", func(t *testing.T) {
		check()
	})
	t.Run("normal", func(t *testing.T) {
		check(_libJsEncrypt())
	})
}

func TestRunWithVariable(t *testing.T) {
	code := `params`
	wantStr := utils.RandStringBytes(32)
	_, value, err := _run(code, _withVariables(map[string]any{
		"params": wantStr,
	}))
	require.NoError(t, err)
	require.Equal(t, wantStr, value.String())
}
