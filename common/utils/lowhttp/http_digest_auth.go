package lowhttp

import (
	"bytes"
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/utils"
)

type DigestRequest struct {
	Body           string
	Method         string
	Password       string
	URI            string
	Username       string
	Auth           *DigestAuthorization
	Wa             *wwwAuthenticate
	CertVal        bool
	useCompleteURL bool
}

// NewDigestRequest creates a new DigestRequest object
func NewDigestRequest(username, password, method, uri, body string, useCompleteURL bool) *DigestRequest {
	dr := &DigestRequest{}
	dr.UpdateRequest(username, password, method, uri, body, useCompleteURL)
	dr.CertVal = true
	return dr
}

func (dr *DigestRequest) UpdateRequest(username, password, method, uri, body string, useCompleteURL bool) *DigestRequest {
	dr.Body = body
	dr.Method = method
	dr.Password = password
	dr.URI = uri
	dr.Username = username
	dr.useCompleteURL = useCompleteURL
	return dr
}

func (dr *DigestRequest) UpdateRequestWithUsernameAndPassword(username, password string) *DigestRequest {
	dr.Username = username
	dr.Password = password
	return dr
}

var (
	algorithmRegex = regexp.MustCompile(`algorithm="([^ ,]+)"`)
	domainRegex    = regexp.MustCompile(`domain="(.+?)"`)
	nonceRegex     = regexp.MustCompile(`nonce="(.+?)"`)
	opaqueRegex    = regexp.MustCompile(`opaque="(.+?)"`)
	qopRegex       = regexp.MustCompile(`qop="(.+?)"`)
	realmRegex     = regexp.MustCompile(`realm="(.+?)"`)
	staleRegex     = regexp.MustCompile(`stale=([^ ,])"`)
	charsetRegex   = regexp.MustCompile(`charset="(.+?)"`)
	userhashRegex  = regexp.MustCompile(`userhash=([^ ,])"`)
)

type DigestAuthorization struct {
	Algorithm string // unquoted
	Cnonce    string // quoted
	Nc        int    // unquoted
	Nonce     string // quoted
	Opaque    string // quoted
	Qop       string // unquoted
	Realm     string // quoted
	Response  string // quoted
	URI       string // quoted
	Userhash  bool   // quoted
	Username  string // quoted
	Username_ string // quoted
}

func newAuthorization(dr *DigestRequest) (*DigestAuthorization, error) {

	ah := DigestAuthorization{
		Algorithm: dr.Wa.Algorithm,
		Cnonce:    "",
		Nc:        0,
		Nonce:     dr.Wa.Nonce,
		Opaque:    dr.Wa.Opaque,
		Qop:       "",
		Realm:     dr.Wa.Realm,
		Response:  "",
		URI:       "",
		Userhash:  dr.Wa.Userhash,
		Username:  "",
		Username_: "", // TODO
	}

	return ah.RefreshAuthorization(dr)
}

const (
	algorithmMD5        = "MD5"
	algorithmMD5Sess    = "MD5-SESS"
	algorithmSHA256     = "SHA-256"
	algorithmSHA256Sess = "SHA-256-SESS"
)

func (ah *DigestAuthorization) RefreshAuthorization(dr *DigestRequest) (*DigestAuthorization, error) {

	ah.Username = dr.Username

	if ah.Userhash {
		ah.Username = ah.hash(fmt.Sprintf("%s:%s", ah.Username, ah.Realm))
	}

	ah.Nc++

	ah.Cnonce = ah.hash(fmt.Sprintf("%d:%s:yak", time.Now().UnixNano(), dr.Username))

	if dr.useCompleteURL {
		ah.URI = dr.URI
	} else {
		u, err := url.Parse(dr.URI)
		if err != nil {
			return nil, err
		}

		ah.URI = u.RequestURI()
	}

	ah.Response = ah.computeResponse(dr)

	return ah, nil
}

func (ah *DigestAuthorization) RefreshAuthorizationWithoutConce(dr *DigestRequest) (*DigestAuthorization, error) {

	ah.Username = dr.Username

	if ah.Userhash {
		ah.Username = ah.hash(fmt.Sprintf("%s:%s", ah.Username, ah.Realm))
	}

	if dr.useCompleteURL {
		ah.URI = dr.URI
	} else {
		u, err := url.Parse(dr.URI)
		if err != nil {
			return nil, err
		}

		ah.URI = u.RequestURI()
	}

	ah.Response = ah.computeResponse(dr)

	return ah, nil
}

func (ah *DigestAuthorization) computeResponse(dr *DigestRequest) (s string) {
	a1 := ah.hash(ah.computeA1(dr))
	a2 := ah.hash(ah.computeA2(dr))
	mid := ""
	if ah.Qop == "auth" || ah.Qop == "auth-int" {
		mid = fmt.Sprintf("%s:%08x:%s:%s", ah.Nonce, ah.Nc, ah.Cnonce, ah.Qop)
	} else {
		mid = ah.Nonce
	}

	return ah.hash(fmt.Sprintf("%s:%s:%s", a1, mid, a2))
}

func (ah *DigestAuthorization) computeA1(dr *DigestRequest) string {

	algorithm := strings.ToUpper(ah.Algorithm)

	if algorithm == "" || algorithm == algorithmMD5 || algorithm == algorithmSHA256 {
		return fmt.Sprintf("%s:%s:%s", ah.Username, ah.Realm, dr.Password)
	}

	if algorithm == algorithmMD5Sess || algorithm == algorithmSHA256Sess {
		upHash := ah.hash(fmt.Sprintf("%s:%s:%s", ah.Username, ah.Realm, dr.Password))
		return fmt.Sprintf("%s:%s:%s", upHash, ah.Nonce, ah.Cnonce)
	}

	return ""
}

func (ah *DigestAuthorization) computeA2(dr *DigestRequest) string {

	if strings.Contains(dr.Wa.Qop, "auth-int") {
		ah.Qop = "auth-int"
		return fmt.Sprintf("%s:%s:%s", dr.Method, ah.URI, ah.hash(dr.Body))
	}

	if dr.Wa.Qop == "auth" || dr.Wa.Qop == "" {
		ah.Qop = dr.Wa.Qop
		return fmt.Sprintf("%s:%s", dr.Method, ah.URI)
	}

	return ""
}

func (ah *DigestAuthorization) hash(a string) string {
	var h hash.Hash
	algorithm := strings.ToUpper(ah.Algorithm)

	if algorithm == "" || algorithm == algorithmMD5 || algorithm == algorithmMD5Sess {
		h = md5.New()
	} else if algorithm == algorithmSHA256 || algorithm == algorithmSHA256Sess {
		h = sha256.New()
	} else {
		// unknown algorithm
		return ""
	}

	io.WriteString(h, a)
	return hex.EncodeToString(h.Sum(nil))
}

func (ah *DigestAuthorization) String() string {
	var buffer bytes.Buffer

	buffer.WriteString("Digest ")

	if ah.Username != "" {
		buffer.WriteString(fmt.Sprintf("username=\"%s\", ", ah.Username))
	}

	if ah.Realm != "" {
		buffer.WriteString(fmt.Sprintf("realm=\"%s\", ", ah.Realm))
	}

	if ah.Nonce != "" {
		buffer.WriteString(fmt.Sprintf("nonce=\"%s\", ", ah.Nonce))
	}

	if ah.URI != "" {
		buffer.WriteString(fmt.Sprintf("uri=\"%s\", ", ah.URI))
	}

	if ah.Response != "" {
		buffer.WriteString(fmt.Sprintf("response=\"%s\", ", ah.Response))
	}

	if ah.Algorithm != "" {
		buffer.WriteString(fmt.Sprintf("algorithm=%s, ", ah.Algorithm))
	}

	if ah.Cnonce != "" {
		buffer.WriteString(fmt.Sprintf("cnonce=\"%s\", ", ah.Cnonce))
	}

	if ah.Opaque != "" {
		buffer.WriteString(fmt.Sprintf("opaque=\"%s\", ", ah.Opaque))
	}

	if ah.Qop != "" {
		buffer.WriteString(fmt.Sprintf("qop=%s, ", ah.Qop))
	}

	if ah.Nc != 0 {
		buffer.WriteString(fmt.Sprintf("nc=%08x, ", ah.Nc))
	}

	if ah.Userhash {
		buffer.WriteString("userhash=true, ")
	}

	s := buffer.String()

	return strings.TrimSuffix(s, ", ")
}

type wwwAuthenticate struct {
	Algorithm string // unquoted
	Domain    string // quoted
	Nonce     string // quoted
	Opaque    string // quoted
	Qop       string // quoted
	Realm     string // quoted
	Stale     bool   // unquoted
	Charset   string // quoted
	Userhash  bool   // quoted
}

func newWWWAuthenticate(s string) *wwwAuthenticate {

	var wa = wwwAuthenticate{}

	algorithmMatch := algorithmRegex.FindStringSubmatch(s)
	if algorithmMatch != nil {
		wa.Algorithm = algorithmMatch[1]
	}

	domainMatch := domainRegex.FindStringSubmatch(s)
	if domainMatch != nil {
		wa.Domain = domainMatch[1]
	}

	nonceMatch := nonceRegex.FindStringSubmatch(s)
	if nonceMatch != nil {
		wa.Nonce = nonceMatch[1]
	}

	opaqueMatch := opaqueRegex.FindStringSubmatch(s)
	if opaqueMatch != nil {
		wa.Opaque = opaqueMatch[1]
	}

	qopMatch := qopRegex.FindStringSubmatch(s)
	if qopMatch != nil {
		wa.Qop = qopMatch[1]
	}

	realmMatch := realmRegex.FindStringSubmatch(s)
	if realmMatch != nil {
		wa.Realm = realmMatch[1]
	}

	staleMatch := staleRegex.FindStringSubmatch(s)
	if staleMatch != nil {
		wa.Stale = (strings.ToLower(staleMatch[1]) == "true")
	}

	charsetMatch := charsetRegex.FindStringSubmatch(s)
	if charsetMatch != nil {
		wa.Charset = charsetMatch[1]
	}

	userhashMatch := userhashRegex.FindStringSubmatch(s)
	if userhashMatch != nil {
		wa.Userhash = (strings.ToLower(userhashMatch[1]) == "true")
	}

	return &wa
}

func GetDigestAuthorizationFromRequestEx(method, url, body, authorization, username, password string, useCompleteURL bool) (*DigestRequest, *DigestAuthorization, error) {
	s := strings.SplitN(authorization, " ", 2)
	if len(s) != 2 || s[0] != "Digest" {
		return nil, nil, utils.Errorf("WWW-Authenticate is not digest auth")
	}

	dr := NewDigestRequest(username, password, method, url, body, useCompleteURL)
	dr.Wa = newWWWAuthenticate(authorization)
	ah, err := newAuthorization(dr)
	if err != nil {
		return nil, nil, err
	}

	return dr, ah, nil
}

func GetDigestAuthorizationFromRequest(raw []byte, authorization, username, password string) (string, error) {
	req, err := ParseBytesToHttpRequest(raw)
	if err != nil {
		return "", err
	}
	body, err := io.ReadAll(req.Body)
	if err != nil {
		return "", err
	}

	splited := strings.Split(utils.UnsafeBytesToString(FixHTTPPacketCRLF(raw, true)), "\r\n")
	_, uri, _, _ := utils.ParseHTTPRequestLine(splited[0])
	useCompleteURL := false
	if strings.Contains(uri, "://") {
		useCompleteURL = true
	}

	_, ah, err := GetDigestAuthorizationFromRequestEx(req.Method, req.URL.String(), utils.UnsafeBytesToString(body), authorization, username, password, useCompleteURL)
	if err != nil {
		return "", err
	}
	return ah.String(), nil
}
