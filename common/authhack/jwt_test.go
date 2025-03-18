package authhack

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/yaklang/yaklang/common/utils/orderedmap"
	"strings"
	"testing"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/dgrijalva/jwt-go"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

func TestJwtParse(t *testing.T) {
	test := assert.New(t)

	spew.Dump(AvailableJWTTokensAlgs())

	jwt.New(jwt.SigningMethodES256)

	str := `eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXUyJ9`
	raw, err := codec.DecodeBase64(str)
	if err != nil {
		test.FailNow("decode header failed: %s", err)
		return
	}
	var res = make(map[string]interface{})
	err = json.Unmarshal(raw, &res)
	if err != nil {
		test.FailNow(err.Error())
		return
	}

	altDes, ok := res["alg"]
	if !ok {
		test.FailNow("no alg in token")
		return
	}
	if !utils.StringArrayContains(AvailableJWTTokensAlgs(), fmt.Sprint(altDes)) {
		test.FailNow("alg error", altDes)
		return
	}

	token, err := NewJWTHelper(fmt.Sprint(altDes))
	if err != nil {
		test.FailNow(err.Error())
		return
	}
	claims := make(jwt.MapClaims)
	claims["test"] = 123
	token.Claims = claims
	tokenStr, err := token.SigningString()
	if err != nil {
		test.FailNow(err.Error())
		return
	}
	println(tokenStr)
}

func TestJwtParse2(t *testing.T) {
	token, secret, err := JwtParse("eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXUyJ9.eyJsb2dpbiI6InRlc3RhYmMiLCJpYXQiOiIxNjM4MzMyMzI5In0.ZDJjYmVkZTJjYmExNzhhYzA2ZWJiZDAwMTJjYmQ1ZTFkOWM4MGE4MDNkNjQxOTgwMWNjNTIwMGEwODgxM2RkNw")
	require.ErrorIs(t, err, ErrKeyNotFound)
	require.NotNil(t, token)
	require.Nil(t, secret)

	testToken, err := JwtGenerate("None", map[string]interface{}{
		"login": "admin",
	}, "JWS", nil)
	require.NoError(t, err)
	token, secret, err = JwtParse(testToken)
	require.NoError(t, err)
	require.NotNil(t, token)
	require.Nil(t, secret)

	spew.Dump(token)
}

func TestJwtParse3(t *testing.T) {
	newToken, err := JwtChangeAlgToNone("eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXUyJ9.eyJsb2dpbiI6InRlc3RhYmMiLCJpYXQiOiIxNjM4MzMyMzI5In0.ZDJjYmVkZTJjYmExNzhhYzA2ZWJiZDAwMTJjYmQ1ZTFkOWM4MGE4MDNkNjQxOTgwMWNjNTIwMGEwODgxM2RkNw")
	if err != nil {
		println(err.Error())
		t.FailNow()
		return
	}
	// eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXUyJ9.eyJpYXQiOiIxNjM4MzMyMzI5IiwibG9naW4iOiJ0ZXN0YWJjIn0.
	// eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXUyJ9.eyJpYXQiOiIxNjM4MzMyMzI5IiwibG9naW4iOiJ0ZXN0YWJjIn0.
	println(newToken)
}

func TestJwtParse4(t *testing.T) {
	newToken, secret, err := JwtParse(
		"eyJ0eXAiOiJKV1QiLCJhbGciOiJSUzI1NiJ9.eyJsb2dpbiI6InRlc3RhYmMifQ.SnEUbh5ykeQFGwvzTHscLr1CurDfRVVUxktT3_G5PIUCueoJdJpTkEf1Z9g4jjfK8Z-rTwwTbHfw8owkQn61alilEMRAwOT5jA9-BVMh90qfBDDTkrLNsT2jfznAGFqDdGzI2Q9KDYSr46_DKobkqqxWvfuJFxYAy3MFyPJAXSE3rF4yGvYD5NLW6mwZgYZvnQARNPhIvJe2UD5IAYjFL82myIi0j4sPLm103qPI6hkQ-7Erv8_1Q_WDiF-Xp3l8OmJbJbMgHfv7sDMxfrvxQfnUCck1Oubq_Vj-1hfuUSbbhS-BQBZUykZ0o9KRVD5uY_bmcEfHbJm2i6eqrf_B3A")
	require.NotNil(t, newToken)
	require.ErrorIs(t, err, ErrKeyNotFound)
	require.Nil(t, secret)
	spew.Dump(newToken)
}

func TestJwtParse5(t *testing.T) {
	token, err := JwtGenerate("HS256", map[string]interface{}{
		"test": "value",
	}, "", []byte("secrjasdfasdfasdfhasdfasdfasdfet"))
	require.NoError(t, err)
	println(token)

	newToken, secret, err := JwtParse(token, WeakJWTTokenKeys...)
	require.ErrorIs(t, err, ErrKeyNotFound)
	require.NotNil(t, newToken)
	require.Nil(t, secret)

	spew.Dump(newToken)
}

func TestJwt(t *testing.T) {
	t.Run("None alg", func(t *testing.T) {
		s, err := JwtGenerate("None", nil, "JWT", nil)
		require.NoError(t, err)
		token, key, err := JwtParse(s)
		require.NoError(t, err)
		require.Nil(t, key)
		require.NotNil(t, token)
	})
	t.Run("invalid alg", func(t *testing.T) {
		s, err := JwtGenerate("None", nil, "JWT", nil)
		require.NoError(t, err)
		splited := strings.SplitN(s, ".", 3)
		decoded, err := base64.RawURLEncoding.DecodeString(splited[0])
		require.NoError(t, err)
		decoded = bytes.Replace(decoded, []byte(`"alg":"None"`), []byte(`"alg":"invalid"`), 1)
		splited[0] = base64.StdEncoding.EncodeToString(decoded)
		s = strings.Join(splited, ".")

		token, key, err := JwtParse(s)
		require.ErrorContains(t, err, "unverifiable token")
		require.Nil(t, key)
		require.NotNil(t, token)

	})
	t.Run("iat", func(t *testing.T) {
		password := uuid.NewString()
		s, err := JwtGenerate("HS256", map[string]any{
			"iat": float64(time.Now().Add(100 * time.Second).Unix()),
		}, "", []byte(password))
		require.NoError(t, err)
		token, key, err := JwtParse(s, password)
		require.ErrorContains(t, err, "token IAT validation failed")
		require.NotNil(t, token)
		require.Equal(t, password, string(key))
	})

	t.Run("exp", func(t *testing.T) {
		password := uuid.NewString()
		s, err := JwtGenerate("HS256", map[string]any{
			"exp": float64(time.Now().Add(-100 * time.Second).Unix()),
		}, "", []byte(password))
		require.NoError(t, err)
		token, key, err := JwtParse(s, password)
		require.ErrorContains(t, err, "token EXP validation failed")
		require.NotNil(t, token)
		require.Equal(t, password, string(key))
	})

	t.Run("nbf", func(t *testing.T) {
		password := uuid.NewString()
		s, err := JwtGenerate("HS256", map[string]any{
			"nbf": float64(time.Now().Add(100 * time.Second).Unix()),
		}, "", []byte(password))
		require.NoError(t, err)
		token, key, err := JwtParse(s, password)
		require.ErrorContains(t, err, "token NBF validation failed")
		require.NotNil(t, token)
		require.Equal(t, password, string(key))
	})

	t.Run("normal", func(t *testing.T) {
		password := uuid.NewString()
		u := uuid.NewString()
		m := map[string]any{"test": u}
		s, err := JwtGenerate("HS256", m, "", []byte(password))
		require.NoError(t, err)
		token, key, err := JwtParse(s, password)
		require.NoError(t, err)
		require.NotNil(t, token)
		require.Equal(t, password, string(key))
		require.Equal(t, m["test"], token.Claims.(*OMapClaims).GetExact("test"))
	})
}

func TestJwtClaimsOrder(t *testing.T) {
	testJWT := `eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIiwibmFtZSI6IkpvaG4gRG9lIiwibW12IjoidHR2IiwienhjIjoicXdlIiwiYWRtaW4iOnRydWUsImlhdCI6MTUxNjIzOTAyMn0.brgCnaG_Aj19IgPTeWdxZN5LZ5lzgrqBMcJmELoCjPQ`

	for i := 0; i < 100; i++ {
		token, _, err := JwtParse(testJWT)
		require.ErrorIs(t, err, ErrKeyNotFound)
		signed, err := token.SignedString([]byte("a-string-secret-at-least-256-bits-long"))
		require.NoError(t, err)
		require.Equal(t, testJWT, signed)
	}
}

func TestJwtGenerateExOrder(t *testing.T) {
	claims := map[string]interface{}{
		"sub":  "test",
		"name": "test",
	}
	c := orderedmap.New(claims)

	// 使用相同的输入参数生成两次
	token1, err1 := JwtGenerateEx("HS256", nil, c, "JWT", []byte("test"))
	token2, err2 := JwtGenerateEx("HS256", nil, c, "JWT", []byte("test"))

	if err1 != nil || err2 != nil {
		t.Fatal(err1, err2)
	}

	// 验证两次生成的 token 是否相同
	if token1 != token2 {
		t.Errorf("Tokens are different: %s != %s", token1, token2)
	}

	_, k, err := JwtParse(token1, "test")
	if err != nil {
		t.Fatal(err)
	}
	if string(k) != "test" {
		t.Errorf("Keys are different: %s != %s", k, "test")
	}
}
