package lowhttp

import (
	"bytes"
	"fmt"
	"github.com/google/shlex"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
)

func CurlToHTTPRequest(i string) ([]byte, error) {
	items, err := shlex.Split(i)
	if err != nil {
		return nil, err
	}

	var (
		method   string = "GET"
		headers         = make(http.Header)
		body            = ""
		urlIndex        = len(items) - 1
	)

	maxItem := len(items)

	// fetch method
	methodIndex := 0
	mustHead := false
	forceGet := false
	for index, item := range items {
		if item == `-I` || item == `--head` {
			mustHead = true
			break
		}

		if item == "-X" || item == "--request" {
			methodIndex = index
		}

		// add a condition for --get or -G
		if item == "-G" || item == "--get" {
			forceGet = true
		}

		if item == "--url" {
			urlIndex = index + 1
		}
	}
	if methodIndex > 0 && methodIndex+1 < maxItem {
		method = items[methodIndex+1]
	}
	if mustHead {
		method = "HEAD"
	}

	// fetch headers
	cookies, err := cookiejar.New(&cookiejar.Options{
		PublicSuffixList: nil,
	})
	if err != nil {
		return nil, utils.Errorf("cookiejar.New failed: %v", err)
	}
	fakeU, _ := url.Parse("http://127.0.0.1")

	var referer string
	var userAgent string
	for index, item := range items {
		// basic user
		if item == "-u" || item == "--user" {
			if index+1 < maxItem {
				headers.Set("Authorization", `Basic `+codec.EncodeBase64(items[index+1]))
			}
		}

		// cookie
		if item == "-b" || item == "--cookie" {
			if index+1 < maxItem {
				cookiesList := splitCookies(items[index+1])
				if len(cookiesList) > 0 {
					cookies.SetCookies(fakeU, cookiesList)
				}
			}
		}

		if item == "-H" || item == "--header" {
			if index+1 < maxItem {
				val := items[index+1]
				if (strings.HasSuffix(val, `"`) && strings.HasPrefix(val, `"`)) || (strings.HasSuffix(val, `'`) && strings.HasPrefix(val, `'`)) {
					var valUnquoted, err = strconv.Unquote(val)
					if err != nil {
						val = val[1 : len(val)-1]
					} else {
						val = valUnquoted
					}
				}
				k, v := SplitHTTPHeader(val)
				if strings.ToLower(k) == "referer" {
					referer = v
				} else if strings.ToLower(k) == "cookie" {
					cookiesList := splitCookies(v)
					cookies.SetCookies(fakeU, cookiesList)
				} else {
					headers.Add(k, v)
				}
			}
		}

		if item == "-e" || item == "--referer" {
			if index+1 < maxItem {
				referer = items[index+1]
			}
		}

		if item == "-A" || item == "--user-agent" {
			if index+1 < maxItem {
				userAgent = items[index+1]
			}
		}
	}

	if referer != "" {
		headers.Set("Referer", referer)
	}

	// -A 或者 --user-agent 应该覆盖 -H 中设置的
	if userAgent != "" {
		headers.Set("User-Agent", userAgent)
	}

	// 合并 -H 设置的 cookie，和 -b 设置的 cookie。
	var cookieStrings []string
	for _, cookie := range cookies.Cookies(fakeU) {
		cookieStrings = append(cookieStrings, cookie.String())
	}
	if len(cookieStrings) != 0 {
		headers.Add("Cookie", strings.Join(cookieStrings, "; "))
	}

	// fetch post body
	data := make(url.Values)
	for index, item := range items {
		if item == "-d" || item == "--data" {
			if index+1 < maxItem {
				body = items[index+1]
				if method == "GET" {
					method = "POST"
				}

				// If forceGet is true, convert data to query parameters
				if forceGet {
					kv := strings.SplitN(body, "=", 2)
					if len(kv) == 2 {
						data.Add(kv[0], kv[1])
					}
				}
			}
		}

		if item == "--data-raw" {
			if index+1 < maxItem {
				body = items[index+1]
				if method == "GET" {
					method = "POST"
				}
			}
		}
	}

	if headers.Get("Content-Type") == "" && body != "" {
		headers.Set("Content-Type", "application/x-www-form-urlencoded")
	}

	// fetch form data
	var formData = make(url.Values)
	for index, item := range items {
		if item == "-F" || item == "--form" {
			if index+1 < maxItem {
				val := items[index+1]
				if (strings.HasSuffix(val, `"`) && strings.HasPrefix(val, `"`)) || (strings.HasSuffix(val, `'`) && strings.HasPrefix(val, `'`)) {
					var valUnquoted, err = strconv.Unquote(val)
					if err != nil {
						val = val[1 : len(val)-1]
					} else {
						val = valUnquoted
					}
				}
				k, v := SplitKV(val)
				formData.Add(k, v)
			}
		}
	}
	if ct := headers.Get("Content-Type"); len(formData) > 0 && (ct == "multipart/form-data" || ct == "") {
		var buf bytes.Buffer
		w := multipart.NewWriter(&buf)
		for k, vs := range formData {
			for _, v := range vs {
				if strings.HasPrefix(v, "@") {
					_, fileName := filepath.Split(v[1:])
					fileWriter, _ := w.CreateFormFile(k, fileName)
					if fileWriter != nil {
						fileWriter.Write([]byte(`{{file(` + v[1:] + `)}}`))
					}
				} else {
					_ = w.WriteField(k, v)
				}
			}
		}
		w.Close()
		body = buf.String()
		headers.Set("Content-Type", w.FormDataContentType())
	}

	targetUrl := items[urlIndex]

	var mayDomain string
	if strings.Contains(targetUrl, "/") {
		mayDomain = strings.Split(targetUrl, "/")[0]
	}

	if strings.HasPrefix(targetUrl, "https://") || strings.HasPrefix(targetUrl, "http://") {
		targetUrl = targetUrl
	} else if utils.IsIPv4(targetUrl) || utils.IsIPv6(targetUrl) || utils.IsValidDomain(targetUrl) {
		targetUrl = "http://" + targetUrl
	} else if ret, err := url.Parse("http://" + targetUrl); err == nil && strings.Contains(mayDomain, `.`) {
		targetUrl = ret.String()
	} else if host, port, err := utils.ParseStringToHostPort(targetUrl); err == nil {
		if port == 443 {
			targetUrl = "https://" + host
		} else if port == 80 {
			targetUrl = "http://" + host
		} else {
			targetUrl = "http://" + utils.HostPort(host, port)
		}
	} else {
		for _, value := range items {
			if strings.Contains(value, " ") || !strings.Contains(value, ".") || (strings.HasPrefix(value, `"`) && strings.HasSuffix(value, `"`) ||
				strings.HasPrefix(value, `'`) && strings.HasSuffix(value, `'`)) {
				continue
			}

			if strings.HasPrefix(value, "https://") || strings.HasPrefix(value, "http://") {
				targetUrl = value
				break
			}

			if strings.Contains(value, "/") {
				firstBlock := strings.Split(value, "/")[0]
				if utils.IsIPv4(firstBlock) || utils.IsIPv6(firstBlock) || utils.IsValidDomain(firstBlock) {
					targetUrl = "http://" + value
					break
				}
			}

			host, port, _ := utils.ParseStringToHostPort(value)
			if port > 0 && host != "" {
				if port == 443 {
					targetUrl = "https://" + host
					break
				}

				if port == 80 {
					targetUrl = "http://" + host
					break
				}
				targetUrl = "http://" + utils.HostPort(host, port)
				break
			}

			if utils.IsIPv4(value) || utils.IsIPv6(value) || utils.IsValidDomain(value) {
				targetUrl = "http://" + host
				break
			}
		}
	}
	if targetUrl == "" {
		return nil, utils.Errorf("invalid url: cannot found url")
	}
	urlIns, err := url.Parse(targetUrl)
	if err != nil {
		return nil, utils.Errorf("invalid url: %v", err)
	}

	if forceGet && len(data) > 0 {
		parsedUrl, err := url.Parse(targetUrl)
		if err != nil {
			return nil, utils.Errorf("invalid url: %v", err)
		}
		parsedUrl.RawQuery = data.Encode()
		targetUrl = parsedUrl.String()
		method = "GET"
	}

	// Fetch the host from the targetUrl
	host := urlIns.Host

	// Set the Host field in headers
	headers.Set("Host", host)

	var headerBuf bytes.Buffer
	for k, v := range headers {
		for _, v1 := range v {
			headerBuf.Write([]byte(fmt.Sprintf("%v: %v\r\n", k, v1)))
		}
	}
	packet := fmt.Sprintf(`%v %v HTTP/1.1
%v
%v`, strings.ToUpper(method), urlIns.RequestURI(), headerBuf.String(), body)
	return FixHTTPRequestOut([]byte(packet)), nil
}

func splitCookies(s string) []*http.Cookie {
	kvPairs := strings.Split(s, ";")
	cookies := make([]*http.Cookie, 0, len(kvPairs))

	for _, kvPair := range kvPairs {
		kvPair = strings.TrimSpace(kvPair) // 去除字符串的开头和结尾的空格
		kv := strings.SplitN(kvPair, "=", 2)
		if len(kv) == 2 {
			cookies = append(cookies, &http.Cookie{
				Name:  strings.TrimSpace(kv[0]),
				Value: strings.TrimSpace(kv[1]),
			})
		}
	}

	return cookies
}
