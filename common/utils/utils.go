package utils

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	uuid "github.com/satori/go.uuid"
	"github.com/yaklang/yaklang/common/gmsm/gmtls"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/progresswriter"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/denisbrodbeck/machineid"
)

func IContains(s, sub string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(sub))
}

func Errorf(origin string, args ...interface{}) error {
	return errors.New(fmt.Sprintf(origin, args...))
}

func Error(i interface{}) error {
	return errors.New(fmt.Sprint(i))
}

func StringLowerAndTrimSpace(raw string) string {
	return strings.ToLower(strings.TrimSpace(raw))
}

func TrimFileNameExt(raw string) string {
	e := filepath.Ext(raw)
	if e == "" {
		return raw
	}

	return strings.Trim(strings.TrimSuffix(raw, e), ". ")
}

func IntArrayContains(array []int, element int) bool {
	for _, s := range array {
		if element == s {
			return true
		}
	}
	return false
}

// BKDR Hash Function
func BKDRHash(str []byte) uint32 {
	var seed uint32 = 131 // 31 131 1313 13131 131313 etc..
	var hash uint32 = 0
	for i := 0; i < len(str); i++ {
		hash = hash*seed + uint32(str[i])
	}

	return hash
}

func SnakeString(s string) string {
	data := make([]byte, 0, len(s)*2)
	j := false
	num := len(s)
	for i := 0; i < num; i++ {
		d := s[i]
		if i > 0 && d >= 'A' && d <= 'Z' && j {
			data = append(data, '_')
		}
		if d != '_' {
			j = true
		}
		data = append(data, d)
	}
	return strings.ToLower(string(data[:]))
}

func TimeoutContext(d time.Duration) context.Context {
	ctx, _ := context.WithTimeout(context.Background(), d)
	return ctx
}

func FloatSecondDuration(f float64) time.Duration {
	return time.Duration(float64(time.Second) * f)
}

func StringAsFileParams(target interface{}) []byte {
	switch ret := target.(type) {
	case string:
		if GetFirstExistedPath(ret) != "" {
			raw, err := ioutil.ReadFile(ret)
			if err != nil {
				return []byte(ret)
			}
			return raw
		} else {
			return []byte(ret)
		}
	case []string:
		return []byte(strings.Join(ret, "\n"))
	case []byte:
		return ret
	case io.Reader:
		raw, err := ioutil.ReadAll(ret)
		if err != nil {
			return nil
		}
		return raw
	default:
		log.Errorf("cannot covnert %v to file content", spew.Sdump(target))
		return nil
	}
}

func TimeoutContextSeconds(d float64) context.Context {
	return TimeoutContext(FloatSecondDuration(d))
}

func GetMachineCode() string {
	mid, err := GetSystemMachineCode()
	if err != nil {
		fileName := filepath.Join(GetHomeDirDefault("."), ".ym-id")
		var raw, _ = ioutil.ReadFile(fileName)
		if raw != nil {
			mid = EscapeInvalidUTF8Byte(raw)
		} else {
			mid = uuid.NewV4().String()
			_ = ioutil.WriteFile(fileName, []byte(mid), 0666)
		}
	}
	return mid
}

func GetSystemMachineCode() (_ string, err error) {
	defer func() {
		if rErr := recover(); rErr != nil {
			err = fmt.Errorf("fetch system machine code failed: %s", rErr)
		}
	}()
	switch runtime.GOOS {
	case "linux":
		raw, err := exec.Command("cat", "/sys/class/dmi/id/product_uuid").CombinedOutput()
		if err != nil {
			unameRaw, _ := exec.Command("uname", "-r").CombinedOutput()
			if bytes.Contains(unameRaw, []byte("microsoft")) {
				raw, err := exec.Command("cat", "/product_uuid").CombinedOutput()
				if err != nil {
					return "", Errorf("please create a file named product_uuid with 36 digs uuid in / path ")
				}
				log.Warnf("wsl detected, uuid: %v", string(raw))
				return codec.EncodeToHex(raw), nil
			} else {
				return "", Errorf("fetch system machine code failed: %s", err)
			}

		}
		return codec.EncodeToHex(raw), nil
	}

	id, _ := machineid.ID()
	if id == "" {
		return "", Errorf("fetch machine code failed")
	}
	//if err != nil {
	//	m := fmt.Sprintf("get machine id failed: %s", err)
	//	return "", errors.New(m)
	//}
	//
	//if id == "" {
	//	return "", Errorf("empty machine-id...")
	//}
	return id, nil
}

func FixJsonRawBytes(rawBytes []byte) []byte {
	rawBytes = []byte(EscapeInvalidUTF8Byte(rawBytes))
	rawBytes = bytes.ReplaceAll(rawBytes, []byte("\\u0000"), []byte(" "))
	return rawBytes
}

func Jsonify(i interface{}) []byte {
	raw, err := json.Marshal(i)
	if err != nil {
		return []byte("{}")
	}
	return raw
}

func NewDefaultHTTPClientWithProxy(proxy string) *http.Client {
	client := NewDefaultHTTPClient()
	if proxy == "" {
		return client
	}
	ht := client.Transport.(*http.Transport)
	ht.Proxy = func(request *http.Request) (*url.URL, error) {
		return url.Parse(proxy)
	}
	return client
}

func DownloadFile(client *http.Client, u string, localFile string, every1s ...func(float64)) error {
	if client == nil {
		return Error("client is nil")
	}

	if GetFirstExistedFile(localFile) != "" {
		return Errorf("localfile: %v is existed", localFile)
	}

	rsp, err := client.Get(u)
	if err != nil {
		return err
	}

	if rsp.Body != nil {
		fp, err := os.OpenFile(localFile, os.O_CREATE|os.O_WRONLY, 0666)
		if err != nil {
			return err
		}
		cl, _ := strconv.Atoi(rsp.Header.Get("Content-Length"))
		if cl <= 0 {
			return Error("content length is 0")
		}
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		pw := progresswriter.New(uint64(cl))
		go func() {
			for {
				select {
				case <-ctx.Done():
					return
				default:
					if len(every1s) > 0 {
						for _, h := range every1s {
							h(pw.GetPercent())
						}
					}
					time.Sleep(time.Second)
				}
			}
		}()
		io.Copy(fp, io.TeeReader(rsp.Body, pw))
		fp.Close()
		return nil
	}
	return Error("body is nil")
}

func NewDefaultHTTPClient() *http.Client {
	jar, _ := cookiejar.New(nil)
	return &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				VerifyPeerCertificate: func(rawCerts [][]byte, verifiedChains [][]*x509.Certificate) error {
					return nil
				},
				InsecureSkipVerify: true,
				MinVersion:         tls.VersionSSL30, // nolint[:staticcheck]
				MaxVersion:         tls.VersionTLS13,
			},
			DisableKeepAlives:  true,
			DisableCompression: true,
			MaxConnsPerHost:    50,
		},
		Timeout: 15 * time.Second,
		Jar:     jar,
	}
}

func NewDefaultTLSClient(conn net.Conn) *tls.Conn {
	return tls.Client(conn, NewDefaultTLSConfig())
}

func NewDefaultTLSConfig() *tls.Config {
	return &tls.Config{
		InsecureSkipVerify: true,
		MinVersion:         tls.VersionSSL30, // nolint[:staticcheck]
		MaxVersion:         tls.VersionTLS13,
	}
}

func NewDefaultGMTLSConfig() *gmtls.Config {
	return &gmtls.Config{
		InsecureSkipVerify: true,
		GMSupport:          &gmtls.GMSupport{},
	}
}

func FixHTTPRequestForHTTPDo(r *http.Request) (*http.Request, error) {
	return FixHTTPRequestForHTTPDoWithHttps(r, false)
}

func FixHTTPRequestForHTTPDoWithHttps(r *http.Request, isHttps bool) (*http.Request, error) {
	var bodyRaw []byte
	var err error
	if r.Body != nil {
		bodyRaw, err = ioutil.ReadAll(r.Body)
		if err != nil {
			return nil, Errorf("read body failed: %s", err)
		}
	}

	if r.URL.Scheme == "" {
		if isHttps {
			r.URL.Scheme = "https"
		} else {
			r.URL.Scheme = "http"
		}
	}

	req, err := http.NewRequest(r.Method, r.URL.String(), bytes.NewBuffer(bodyRaw))
	if err != nil {
		return nil, Errorf("build http.Request[%v] failed: %v", r.URL.String(), err)
	}

	if req.Host == "" && r.Host != "" {
		req.Host = r.Host
	}

	for key, values := range r.Header {
		req.Header[key] = values
	}

	//respect proto
	req.Proto = r.Proto
	req.ProtoMajor = r.ProtoMajor
	req.ProtoMinor = r.ProtoMinor
	return req, nil
}
