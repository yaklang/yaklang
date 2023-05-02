package lowhttp

import (
	"bytes"
	"fmt"
	"github.com/google/shlex"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

func CurlToHTTPRequest(i string) ([]byte, error) {
	items, err := shlex.Split(i)
	if err != nil {
		return nil, err
	}

	var (
		method  string = "GET"
		headers        = make(http.Header)
		body           = ""
	)

	maxItem := len(items)

	// fetch method
	methodIndex := 0
	mustHead := false
	for index, item := range items {
		if item == `-I` || item == `--head` {
			mustHead = true
			break
		}

		if item == "-X" || item == "--request" {
			methodIndex = index
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
				k, v := SplitKV(items[index+1])
				cookies.SetCookies(fakeU, []*http.Cookie{
					{Name: k, Value: v},
				})
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
				headers.Add(k, v)
			}
		}
	}

	for _, cookie := range cookies.Cookies(fakeU) {
		headers.Add("Cookie", cookie.String())
	}

	// fetch post body
	for index, item := range items {
		if item == "-d" || item == "--data" {
			if index+1 < maxItem {
				body = items[index+1]
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

	targetUrl := items[len(items)-1]

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
