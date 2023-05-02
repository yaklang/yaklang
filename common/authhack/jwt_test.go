package authhack

import (
	"encoding/json"
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"github.com/dgrijalva/jwt-go"
	"github.com/stretchr/testify/assert"
	"testing"
	"yaklang/common/utils"
	"yaklang/common/yak/yaklib/codec"
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
	if err != nil {
		spew.Dump(err)
		t.FailNow()
	}
	spew.Dump(token, secret)

	testToken, err := JwtGenerate("None", map[string]interface{}{
		"login": "admin",
	}, "JWS", nil)
	if err != nil {
		spew.Dump(err)
		t.FailNow()
		return
	}
	println(testToken)

	// eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXUyJ9.eyJpYXQiOiIxIiwibG9naW4iOiJhZG1pbiJ9.am57cdFRRffycP0Wr5OC9Ron18N7YP8431rZCESAJiQ
	// eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXUyJ9.eyJsb2dpbiI6InRlc3RhYmMiLCJpYXQiOiIxNjM4MzMyMzI5In0.ZDJjYmVkZTJjYmExNzhhYzA2ZWJiZDAwMTJjYmQ1ZTFkOWM4MGE4MDNkNjQxOTgwMWNjNTIwMGEwODgxM2RkNw

	token, _, err = JwtParse(testToken)
	if err != nil {
		println(err.Error())
		t.FailNow()
	}
	spew.Dump(token)
	_ = token
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
	test := assert.New(t)
	newToken, secret, err := JwtParse(
		"eyJ0eXAiOiJKV1QiLCJhbGciOiJSUzI1NiJ9.eyJsb2dpbiI6InRlc3RhYmMifQ.SnEUbh5ykeQFGwvzTHscLr1CurDfRVVUxktT3_G5PIUCueoJdJpTkEf1Z9g4jjfK8Z-rTwwTbHfw8owkQn61alilEMRAwOT5jA9-BVMh90qfBDDTkrLNsT2jfznAGFqDdGzI2Q9KDYSr46_DKobkqqxWvfuJFxYAy3MFyPJAXSE3rF4yGvYD5NLW6mwZgYZvnQARNPhIvJe2UD5IAYjFL82myIi0j4sPLm103qPI6hkQ-7Erv8_1Q_WDiF-Xp3l8OmJbJbMgHfv7sDMxfrvxQfnUCck1Oubq_Vj-1hfuUSbbhS-BQBZUykZ0o9KRVD5uY_bmcEfHbJm2i6eqrf_B3A",
	)
	if err != nil {
		test.FailNow(err.Error())
	}
	println(string(secret))
	spew.Dump(newToken)
}

func TestJwtParse5(t *testing.T) {
	test := assert.New(t)
	token, err := JwtGenerate("HS256", map[string]interface{}{
		"test": "value",
	}, "", []byte("secrjasdfasdfasdfhasdfasdfasdfet"))
	if err != nil {
		test.FailNow(err.Error())
		return
	}
	println(token)

	tokenIns, secret, err := JwtParse(token)
	if err != nil {
		test.FailNow(err.Error())
		return
	}
	println(string(secret))
	spew.Dump(tokenIns)
}
