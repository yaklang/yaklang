package twofa

import (
	"encoding/base32"
	"net/url"
	"rsc.io/qr"
	"strings"
)

func GenerateQRCode(name, account string, token string) (*url.URL, []byte, error) {
	urlBase, _ := url.Parse("otpauth://totp")
	urlBase.Path += "/" + url.PathEscape(name) + ":" + url.PathEscape(account)
	params := url.Values{}
	params.Add("secret", base32.StdEncoding.EncodeToString([]byte(token)))
	params.Add("issuer", name)
	urlBase.RawQuery = params.Encode()
	code, err := qr.Encode(strings.TrimSpace(urlBase.String()), qr.Q)
	if err != nil {
		return nil, nil, err
	}
	b := code.PNG()
	return urlBase, b, nil
}
