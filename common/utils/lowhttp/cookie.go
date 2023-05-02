package lowhttp

import (
	"bufio"
	"bytes"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/cookiejar"
	"strings"
	"sync"
	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/utils"
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

func CookiesToString(cookies []*http.Cookie) string {
	results := funk.Map(cookies, func(cookie *http.Cookie) string {
		return cookie.String()
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
