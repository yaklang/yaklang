package lowhttp

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"sort"
	"strings"

	"github.com/corpix/uarand"
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/multipart"
	"github.com/yaklang/yaklang/common/utils/shlex"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

var urlPrefixPattern = regexp.MustCompile(`^(https?|wss?)://`)

type Curl struct {
	Headers        http.Header
	URLInstance    *url.URL
	dataPairs      *QueryParams
	formPairs      *QueryParams
	Method         string
	Protocol       string
	uploadFile     string
	body           []byte
	IsTLS          bool
	putDataInQuery bool
	parsed         bool
}

func NewCurlStruct() *Curl {
	return &Curl{
		Method:   "",
		Protocol: "HTTP/1.1",
		Headers:  make(http.Header),
	}
}

type maybeURL struct {
	u           string
	credibility uint8
}

func NewMaybeURL(u string, credibility uint8) *maybeURL {
	return &maybeURL{
		u:           u,
		credibility: credibility,
	}
}

func toCurlURL(u string) (*maybeURL, bool) {
	if urlPrefixPattern.MatchString(u) {
		return NewMaybeURL(u, 4), true
	} else if utils.IsIPv4(u) || utils.IsIPv6(u) || utils.IsValidDomain(u) {
		u = "http://" + u
		return NewMaybeURL(u, 3), true
	} else if ret, err := url.Parse("http://" + u); err == nil && strings.Contains(u, `.`) {
		u = ret.String()
		return NewMaybeURL(u, 2), true
	} else if host, port, err := utils.ParseStringToHostPort(u); err == nil {
		if port == 443 {
			u = "https://" + host
		} else if port == 80 {
			u = "http://" + host
		} else {
			u = "http://" + utils.HostPort(host, port)
		}
		return NewMaybeURL(u, 1), true
	}
	return nil, false
}

func safeIndex(args []string, i int) (string, bool) {
	if i >= len(args) {
		return "", false
	}
	return args[i], true
}

func (c *Curl) ParseFromRaw(raw string) error {
	args, err := shlex.Split(raw)
	if err != nil {
		return err
	}
	args = args[1:]
	args = lo.FilterMap(args, func(s string, _ int) (string, bool) {
		// no trim space for quoted string
		if index := strings.Index(raw, s); index != -1 && index-1 >= 0 && (raw[index-1] == '\'' || raw[index-1] == '"') {
			return s, true
		}
		s = strings.TrimSpace(s)
		return s, s != ""
	})

	return c.ParseOptions(args)
}

func (c *Curl) ParseOptions(args []string) error {
	// options
	cookies, _ := cookiejar.New(&cookiejar.Options{
		PublicSuffixList: nil,
	})
	fakeURLIns, _ := url.Parse("http://example.com")
	maybeURLs := make([]*maybeURL, 0, 1)
	var (
		dataPairs *QueryParams
		formPairs *QueryParams
	)

	for i := 0; i < len(args); i++ {
		switch arg := args[i]; arg {
		case `-I`, `--head`:
			c.Method = "HEAD"
		case `-X`, `--request`:
			if method, ok := safeIndex(args, i+1); ok {
				i++
				c.Method = method
			} else {
				return utils.Error("missing method in --request")
			}
		case `-G`, `--get`:
			c.putDataInQuery = true
		case `-u`, `--user`:
			if user, ok := safeIndex(args, i+1); ok {
				i++
				c.Headers.Add("Authorization", "Basic "+codec.EncodeBase64(user))
			} else {
				return utils.Error("missing user in --user")
			}
		case `-b`, `--cookie`:
			if cookie, ok := safeIndex(args, i+1); ok {
				i++
				cookies.SetCookies(fakeURLIns, splitCookies(cookie))
			} else {
				return utils.Error("missing cookie in --cookie")
			}
		case `-H`, `--header`:
			if header, ok := safeIndex(args, i+1); ok {
				i++
				k, v := SplitHTTPHeader(header)
				if strings.ToLower(k) == "cookie" {
					cookiesList := splitCookies(v)
					cookies.SetCookies(fakeURLIns, cookiesList)
				} else {
					c.Headers.Add(k, v)
				}
			} else {
				return utils.Error("missing header in --header")
			}
		case `-e`, `--referer`:
			if referer, ok := safeIndex(args, i+1); ok {
				i++
				c.Headers.Add("Referer", referer)
			} else {
				return utils.Error("missing referer in --referer")
			}
		case `-A`, `--user-agent`:
			if userAgent, ok := safeIndex(args, i+1); ok {
				i++
				c.Headers.Add("User-Agent", userAgent)
			} else {
				return utils.Error("missing user-agent in --user-agent")
			}
		case `-r`, `--range`:
			if rangeStr, ok := safeIndex(args, i+1); ok {
				i++
				c.Headers.Add("Range", fmt.Sprintf("bytes=%s", rangeStr))
			} else {
				return utils.Error("missing range in --range")
			}
		case `-d`, `--data`, `--data-raw`, `--data-binary`, `--data-urlencode`:
			if data, ok := safeIndex(args, i+1); ok {
				i++
				if c.dataPairs == nil {
					c.dataPairs = NewQueryParams()
					dataPairs = c.dataPairs
					dataPairs.DisableAutoEncode(true)
				}
				if arg == `--data-binary` && strings.HasPrefix(data, "@") {
					dataPairs.AppendRaw(fmt.Sprintf("{{file(%s)}}", data[1:]))
				} else {
					k, v := SplitKV(data)
					if arg == `--data-urlencode` {
						dataPairs.DisableAutoEncode(false)
					}
					if v == "" {
						dataPairs.AppendRaw(k)
					} else {
						dataPairs.Add(k, v)
					}
					if arg == `--data-urlencode` {
						dataPairs.DisableAutoEncode(true)
					}
				}
			} else {
				return utils.Error("missing data in --data")
			}
		case `-T`, `--upload-file`:
			if file, ok := safeIndex(args, i+1); ok {
				i++
				c.body = []byte(fmt.Sprintf("{{file(%s)}}", file))
				c.uploadFile = file
			} else {
				return utils.Error("missing file in --upload-file")
			}
		case `-F`, `--form`, `--form-string`:
			if form, ok := safeIndex(args, i+1); ok {
				i++
				if c.formPairs == nil {
					c.formPairs = NewQueryParams()
					formPairs = c.formPairs
					formPairs.DisableAutoEncode(true)
				}
				k, v := SplitKV(form)
				formPairs.Add(k, v)
			} else {
				return utils.Error("missing form in --form")
			}
		case `--url`:
			if u, ok := safeIndex(args, i+1); ok {
				i++
				if maybeURL, ok := toCurlURL(u); ok {
					maybeURL.credibility = 5
					maybeURLs = append(maybeURLs, maybeURL)
				} else {
					return utils.Error("invalid url in --url")
				}
			} else {
				return utils.Error("missing url in --url")
			}
		case `-0`, `--http1.0`:
			c.Protocol = "HTTP/1.0"
		case `--http1.1`:
			c.Protocol = "HTTP/1.1"
		case `--http2`, `http2-prior-knowledge`:
			c.Protocol = "HTTP/2"
		case `--http3`:
			c.Protocol = "HTTP/3"
		case `--compressed`:
			c.Headers.Add("Accept-Encoding", "gzip, deflate, br")
		case `--abstract-unix-socket`, `--alt-svc`, `--cacert`, `--capath`, `-E`, `--cert`, `--cert-type`, `--ciphers`, `-K`, `--config`, `--connect-timeout`, `--connect-to`, `-C`, `--continue-at`, `-c`, `--cookie-jar`, `--crlfile`, `--data-ascii`, `--delegation`, `--dns-interface`, `--dns-ipv4-addr`, `--dns-ipv6-addr`, `--dns-servers`, `--doh-url`, `-D`, `--dump-header`, `--egd-file`, `--engine`, `--etag-save`, `--etag-compare`, `--expect100-timeout`, `--ftp-account`, `--ftp-alternative-to-user`, `--ftp-method`, `-P`, `--ftp-port`, `--ftp-ssl-ccc-mode`, `--happy-eyeballs-timeout-ms`, `--hostpubmd5`, `--interface`, `--keepalive-time`, `--key`, `--key-type`, `--krb`, `--libcurl`, `--limit-rate`, `--local-port`, `--login-options`, `--mail-auth`, `--mail-from`, `--mail-rcpt`, `--max-filesize`, `--max-redirs`, `-m`, `--max-time`, `--netrc-file`, `--noproxy`, `--oauth2-bearer`, `-o`, `--output`, `--pass`, `--pinnedpubkey`, `--proto`, `--proto-default`, `--proto-redir`, `--proxy-cacert`, `--proxy-capath`, `--proxy-cert`, `--proxy-cert-type`, `--proxy-ciphers`, `--proxy-crlfile`, `--proxy-header`, `--proxy-key`, `--proxy-key-type`, `--proxy-pass`, `--proxy-pinnedpubkey`, `--proxy-service-name`, `--proxy-tls13-ciphers`, `--proxy-tlsauthtype`, `--proxy-tlspassword`, `--proxy-tlsuser`, `-U`, `--proxy-user`, `--proxy1.0`, `--pubkey`, `--random-file`, `--retry`, `--retry-delay`, `--retry-max-time`, `--sasl-authzid`, `--service-name`, `--socks4`, `--socks4a`, `--socks5`, `--socks5-gssapi-service`, `-Y`, `--speed-limit`, `-y`, `--speed-time`, `-t`, `--telnet-option`, `--tftp-blksize`, `-z`, `--time-cond`, `--tls-max`, `--tls13-ciphers`, `--tlsauthtype`, `--tlsuser`, `--trace`, `--unix-socket`, `-w`, `--write-out`:
			// ignore but has argument value
			i++
		case `--anyauth`, `-a`, `--append`, `--basic`, `--cert-status`, `--compressed-ssh`, `--create-dirs`, `--crlf`, `--data-raw <data> HTTP POST data`, `'@'`, `--digest`, `-q`, `--disable`, `--disable-eprt`, `--disable-epsv`, `--disallow-username-in-url`, `-f`, `--fail`, `--fail-early    Fail on first transfer error`, `do`, `--false-start`, `--ftp-create-dirs`, `--ftp-pasv`, `--ftp-pret`, `--ftp-skip-pasv-ip`, `--ftp-ssl-ccc`, `--ftp-ssl-control Require SSL/TLS for FTP login`, `clear`, `-g`, `--globoff`, `--haproxy-protocol`, `-h`, `--help`, `--http0.9`, `--http2-prior-knowledge`, `--ignore-content-length`, `-i`, `--include`, `-k`, `--insecure`, `-4`, `--ipv4`, `-6`, `--ipv6`, `-j`, `--junk-session-cookies`, `-l`, `--list-only`, `-L`, `--location`, `--location-trusted Like --location`, `and`, `-M`, `--manual`, `--metalink`, `--negotiate`, `-n`, `--netrc`, `--netrc-optional`, `-:`, `--next`, `--no-alpn`, `-N`, `--no-buffer`, `--no-keepalive`, `--no-npn`, `--no-progress-meter`, `--no-sessionid`, `--ntlm`, `--ntlm-wb`, `-Z`, `--parallel`, `--parallel-immediate`, `--parallel-max`, `--path-as-is`, `--post301`, `--post302`, `--post303`, `--preproxy`, `-#`, `--progress-bar`, `-x`, `--proxy`, `--proxy-anyauth`, `--proxy-basic`, `--proxy-digest`, `--proxy-insecure`, `--proxy-negotiate`, `--proxy-ntlm`, `--proxy-ssl-allow-beast`, `--proxy-tlsv1`, `-p`, `--proxytunnel`, `-Q`, `--quote`, `--raw`, `-J`, `--remote-header-name`, `-O`, `--remote-name`, `--remote-name-all`, `-R`, `--remote-time`, `--request-target`, `--resolve <host:port:address[`, `address]...>`, `--retry-connrefused`, `--sasl-ir`, `-S`, `--show-error`, `-s`, `--silent`, `--socks5-basic`, `--socks5-gssapi`, `--socks5-gssapi-nec`, `--socks5-hostname <host[:port]> SOCKS5 proxy`, `pass`, `--ssl`, `--ssl-allow-beast`, `--ssl-no-revoke`, `--ssl-reqd`, `-2`, `--sslv2`, `-3`, `--sslv3`, `--stderr`, `--styled-output`, `--suppress-connect-headers`, `--tcp-fastopen`, `--tcp-nodelay`, `--tftp-no-options`, `--tlspassword`, `-1`, `--tlsv1`, `--tlsv1.0`, `--tlsv1.1`, `--tlsv1.2`, `--tlsv1.3`, `--tr-encoding`, `--trace-ascii <file> Like --trace`, `but`, `--trace-time`, `-B`, `--use-ascii`, `-v`, `--verbose`, `-V`, `--version`, `--xattr`:

		default:
			if strings.HasPrefix(arg, "-") {
				continue
			}
			if u, ok := toCurlURL(arg); ok {
				maybeURLs = append(maybeURLs, u)
			}
		}
	}

	// method
	if c.Method == "" {
		if c.uploadFile != "" {
			c.Method = "PUT"
		} else if !c.putDataInQuery && (!c.dataPairs.IsEmpty() || !c.formPairs.IsEmpty()) {
			c.Method = "POST"
		} else {
			c.Method = "GET"
		}
	}
	// url
	if len(maybeURLs) == 0 {
		return utils.Error("missing url")
	}
	sort.SliceStable(maybeURLs, func(i, j int) bool {
		return maybeURLs[i].credibility > maybeURLs[j].credibility
	})
	uIns, err := url.Parse(maybeURLs[0].u)
	if err != nil {
		return err
	}
	c.IsTLS = uIns.Scheme == "https" || uIns.Scheme == "wss"
	if c.uploadFile != "" && (uIns.Path == "/" || uIns.Path == "") {
		uIns.Path = "/" + c.uploadFile
	}
	c.URLInstance = uIns

	// merge cookies
	if cookiesSlice := cookies.Cookies(fakeURLIns); len(cookiesSlice) != 0 {
		c.Headers.Set("Cookie", CookieToNative(cookiesSlice))
	}
	c.parsed = true

	return nil
}

func (c *Curl) ToRawHTTPRequest() ([]byte, error) {
	if !c.parsed {
		return nil, utils.Error("curl not parsed")
	}

	// headers
	// expect 100-continue if upload file and has data
	if c.uploadFile != "" {
		if !c.dataPairs.IsEmpty() || !c.formPairs.IsEmpty() {
			c.Headers.Set("Expect", "100-continue")
		}
	}
	// if not set user-agent, set random user-agent
	if c.Headers.Get("User-Agent") == "" {
		c.Headers.Set("User-Agent", uarand.GetRandom())
	}
	// if not Accept, set */*
	if c.Headers.Get("Accept") == "" {
		c.Headers.Set("Accept", "*/*")
	}

	// body
	var body []byte
	if len(c.body) > 0 {
		body = c.body
	} else if !c.formPairs.IsEmpty() {
		var buf bytes.Buffer
		w := multipart.NewWriter(&buf)
		for _, item := range c.formPairs.Items {
			k, v := item.Key, item.Value
			if strings.HasPrefix(v, "@") {
				v = v[1:]
				if f, err := w.CreateFormFile(k, v); err == nil {
					f.Write([]byte(fmt.Sprintf("{{file(%s)}}", v)))
				}
			} else {
				w.WriteField(k, v)
			}
		}
		w.Close()
		body = buf.Bytes()
		c.Headers.Set("Content-Type", w.FormDataContentType())
	} else if !c.dataPairs.IsEmpty() {
		if c.putDataInQuery {
			c.URLInstance.RawQuery = c.dataPairs.Encode()
		} else {
			if c.Headers.Get("Content-Type") == "" {
				c.Headers.Set("Content-Type", "application/x-www-form-urlencoded")
			}
			body = []byte(c.dataPairs.Encode())
		}
	}
	c.body = body

	// handle host
	if c.Headers.Get("Host") == "" {
		c.Headers.Set("Host", c.URLInstance.Host)
	}
	var headerBuf bytes.Buffer
	for k, v := range c.Headers {
		for _, v1 := range v {
			headerBuf.Write([]byte(fmt.Sprintf("%v: %v\r\n", k, v1)))
		}
	}

	// chunked
	if c.Headers.Get("Transfer-Encoding") == "chunked" {
		body = codec.HTTPChunkedEncode(body)
	}
	if len(body) > 0 {
		body = append([]byte("\r\n"), body...)
	}
	packet := []byte(fmt.Sprintf("%v %v %s\r\n%v%s", strings.ToUpper(c.Method), c.URLInstance.RequestURI(), c.Protocol, headerBuf.String(), body))

	return FixHTTPRequest(packet), nil
}

func CurlToRawHTTPRequest(i string) ([]byte, error) {
	curl := NewCurlStruct()
	err := curl.ParseFromRaw(i)
	if err != nil {
		return nil, err
	}
	return curl.ToRawHTTPRequest()
}
