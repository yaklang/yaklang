package chatglm

import (
	"errors"
	"strings"
	"time"

	"github.com/dgrijalva/jwt-go"
)

const (
	APITokenTTLSeconds = 3 * 60
	APIKeyPartCount    = 2
)

func generateToken(apikey string) (string, error) {
	parts := strings.Split(apikey, ".")
	if len(parts) != APIKeyPartCount {
		return "", errors.New("invalid apikey")
	}
	id := parts[0]
	secret := parts[1]

	payload := jwt.MapClaims{
		"api_key":   id,
		"exp":       time.Now().Add(time.Second * APITokenTTLSeconds).Unix(),
		"timestamp": time.Now().Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, payload)
	token.Header["alg"] = "HS256"
	token.Header["sign_type"] = "SIGN"

	signedToken, err := token.SignedString([]byte(secret))
	if err != nil {
		return "", err
	}

	return signedToken, nil
}
