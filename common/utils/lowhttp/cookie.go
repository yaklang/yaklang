package lowhttp

import (
	"bufio"
	"bytes"
	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strconv"
	"strings"
	"sync"
)

var CookiejarPool sync.Map

func GetCookiejar(session interface{}) http.CookieJar {
	var jar http.CookieJar

	if iJar, ok := CookiejarPool.Load(session); !ok {
		jar, _ = cookiejar.New(nil)
		CookiejarPool.Store(session, jar)
	} else {
		jar = iJar.(http.CookieJar)
	}

	return jar
}

func ExtractCookieJarFromHTTPResponse(rawResponse []byte) []*http.Cookie {
	rsp, err := ParseStringToHTTPResponse(string(rawResponse))
	if err != nil {
		return nil
	}

	return rsp.Cookies()
}

func CookieSafeQuoteString(i string) string {
	if ret, ok := utils.IsJSON(i); ok {
		return url.QueryEscape(ret)
	} else if strings.ContainsAny(i, " ,") {
		return `"` + url.QueryEscape(i) + `"`
	} else {
		return i
	}
}

func CookieSafeUnquoteString(i string) string {
	if strings.HasPrefix(i, `"`) && strings.HasSuffix(i, `"`) {
		i = i[1 : len(i)-1]
	}
	if ret, err := url.QueryUnescape(i); err == nil {
		return ret
	}
	return i
}

// CookieTimeFormat is the time format to use when generating times in HTTP
// headers. It is like time.RFC1123 but hard-codes GMT as the time
// zone. The time being formatted must be in UTC for Format to
// generate the correct format.
//
// For parsing this time format, see ParseTime.
const CookieTimeFormat = "Mon, 02 Jan 2006 15:04:05 GMT"

func CookiesToString(cookies []*http.Cookie, encoded ...codec.EncodedFunc) string {
	results := funk.Map(cookies, func(c *http.Cookie) string {
		var b strings.Builder
		b.Grow(len(c.Name) + len(c.Value) + len(c.Domain) + len(c.Path) + 110 /*RFC 6265 Sec 4.1. extraCookieLength*/)
		b.WriteString(url.QueryEscape(c.Name))
		b.WriteRune('=')
		b.WriteString(CookieSafeQuoteString(c.Value))
		if len(c.Path) > 0 {
			b.WriteString("; Path=")
			b.WriteString(CookieSafeQuoteString(c.Path))
		}

		if len(c.Domain) > 0 {
			b.WriteString("; Domain=")
			b.WriteString(CookieSafeQuoteString(strings.TrimLeft(c.Domain, ".")))
		}

		var buf [len(CookieTimeFormat)]byte
		if !c.Expires.IsZero() {
			b.WriteString("; Expires=")
			b.Write(c.Expires.UTC().AppendFormat(buf[:0], CookieTimeFormat))
		}
		if c.MaxAge > 0 {
			b.WriteString("; Max-Age=")
			b.Write(strconv.AppendInt(buf[:0], int64(c.MaxAge), 10))
		} else if c.MaxAge < 0 {
			b.WriteString("; Max-Age=0")
		}
		if c.HttpOnly {
			b.WriteString("; HttpOnly")
		}
		if c.Secure {
			b.WriteString("; Secure")
		}
		switch c.SameSite {
		case http.SameSiteDefaultMode:
			// Skip, default mode is obtained by not emitting the attribute.
		case http.SameSiteNoneMode:
			b.WriteString("; SameSite=None")
		case http.SameSiteLaxMode:
			b.WriteString("; SameSite=Lax")
		case http.SameSiteStrictMode:
			b.WriteString("; SameSite=Strict")
		}
		return b.String()
	})
	return strings.Join(results.([]string), "; ")
}

func AddOrUpgradeCookie(raw []byte, value string) ([]byte, error) {
	var writer bytes.Buffer

	raw = TrimLeftHTTPPacket(raw)
	reader := bufio.NewReader(bytes.NewBuffer(raw))
	firstLineBytes, err := utils.BufioReadLine(reader)
	if err != nil {
		return nil, err
	}
	writer.Write(firstLineBytes)
	writer.WriteString(CRLF)

	isHeaderExist := false

	for {
		lineBytes, err := utils.BufioReadLine(reader)
		if err != nil && err != io.EOF {
			break
		}
		if bytes.TrimSpace(lineBytes) == nil {
			break
		}

		if strings.HasPrefix(strings.ToLower(string(lineBytes)), "cookie:") { // upgrade cookie
			isHeaderExist = true
			writer.Write(lineBytes)
			writer.WriteString("; " + value)
			writer.WriteString(CRLF)
		} else {
			writer.Write(lineBytes)
			writer.WriteString(CRLF)
		}
	}

	// 如果不存在则添加请求头
	if !isHeaderExist {
		writer.WriteString("Cookie: " + value + CRLF)
	}
	writer.WriteString(CRLF)

	bodyRaw, _ := ioutil.ReadAll(reader)

	if bytes.HasSuffix(bodyRaw, []byte(CRLF+CRLF)) {
		bodyRaw = bodyRaw[:len(bodyRaw)-4]
	}

	if bodyRaw == nil {
		return writer.Bytes(), nil
	}

	// 单独修复请求中的问题
	if !strings.HasPrefix(string(firstLineBytes), "HTTP/") {
		if bytes.HasSuffix(bodyRaw, []byte("\n\n")) {
			bodyRaw = bodyRaw[:len(bodyRaw)-2]
		}
	}

	writer.Write(bodyRaw)
	return writer.Bytes(), nil
}
