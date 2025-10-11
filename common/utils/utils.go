package utils

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
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

	"github.com/gobwas/glob"
	"github.com/jinzhu/gorm"
	_ "github.com/mattn/go-sqlite3"

	uuid "github.com/google/uuid"
	"github.com/yaklang/yaklang/common/gmsm/gmtls"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/progresswriter"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"

	"github.com/davecgh/go-spew/spew"
	"github.com/denisbrodbeck/machineid"
)

// LowerAndTrimSpace 将字符串raw转换为小写并去除前后空白字符
// Example:
// ```
// str.LowerAndTrimSpace("  Hello  ") // "hello"
// ```
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

func IContains(s, sub string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(sub))
}

func IHasPrefix(s, sub string) bool {
	return strings.HasPrefix(strings.ToLower(s), strings.ToLower(sub))
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

func GetContextKeyString(ctx context.Context, key string) string {
	if ctx == nil {
		return ""
	}
	if v := ctx.Value(key); v != nil {
		return InterfaceToString(v)
	}
	return ""
}

func GetContextKeyBool(ctx context.Context, key string) bool {
	if ctx == nil {
		return false
	}
	if v := ctx.Value(key); v != nil {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return false
}

func SetContextKey(ctx context.Context, key string, value any) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if value == nil {
		return ctx
	}
	return context.WithValue(ctx, key, value)
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
		raw, _ := ioutil.ReadFile(fileName)
		if raw != nil {
			mid = EscapeInvalidUTF8Byte(raw)
		} else {
			mid = uuid.New().String()
			_ = ioutil.WriteFile(fileName, []byte(mid), 0o666)
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

func MustUnmarshalJson[T any](raw []byte) *T {
	var ret T
	if err := json.Unmarshal(raw, &ret); err != nil {
		return nil
	}
	return &ret
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

func DownloadFile(ctx context.Context, client *http.Client, u string, localFile string, every1s ...func(float64)) error {
	if client == nil {
		return Error("client is nil")
	}

	if GetFirstExistedFile(localFile) != "" {
		return Errorf("localfile: %v is existed", localFile)
	}

	subCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	request, err := http.NewRequestWithContext(subCtx, http.MethodGet, u, nil) // http with context will cancel it himself
	if err != nil {
		return err
	}

	rsp, err := client.Do(request)
	if err != nil {
		return err
	}

	if rsp.Body != nil {
		defer rsp.Body.Close()
		fp, err := os.OpenFile(localFile, os.O_CREATE|os.O_WRONLY, 0o666)
		if err != nil {
			return err
		}
		defer fp.Close()

		cl := 0
		clHeader := rsp.Header.Get("Content-Length")
		if clHeader != "" {
			cl, _ = strconv.Atoi(clHeader) // should can process chunk
		}
		pw := progresswriter.New(uint64(cl))
		go func() {
			for {
				select {
				case <-subCtx.Done():
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

		_, err = io.Copy(fp, io.TeeReader(rsp.Body, pw))
		if err != nil { // maybe can delete file?
			log.Errorf("download file failed: %v", err)
			return err
		}
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
				Renegotiation:      tls.RenegotiateFreelyAsClient,
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
		Renegotiation:      tls.RenegotiateFreelyAsClient,
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

	// respect proto
	req.Proto = r.Proto
	req.ProtoMajor = r.ProtoMajor
	req.ProtoMinor = r.ProtoMinor
	return req, nil
}

func CallWithCtx(ctx context.Context, cb func()) error {
	sig := make(chan struct{})
	go func() {
		cb()
		sig <- struct{}{}
	}()
	select {
	case <-sig:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func CallWithTimeout(timeout float64, cb func()) error {
	ctx, cancel := context.WithCancel(TimeoutContextSeconds(timeout))
	defer cancel()
	sig := make(chan struct{})
	go func() {
		cb()
		sig <- struct{}{}
	}()
	select {
	case <-sig:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func HostContains(rule string, target string) bool {
	rule = strings.TrimRight(rule, ":")
	target = strings.TrimRight(target, ":")
	ruleHost, rulePort, err := ParseStringToHostPort(rule)
	if err != nil {
		ruleHost = rule
	}
	targetHost, targetPort, err := ParseStringToHostPort(target)
	if err != nil {
		targetHost = target
	}
	_, netBlock, err := net.ParseCIDR(rule) // 尝试解CIDR
	if err == nil && netBlock != nil {
		return netBlock.Contains(net.ParseIP(targetHost))
	}
	if !(rulePort == targetPort || rulePort == 0) {
		return false
	}
	globRuler, err := glob.Compile(ruleHost)
	if err != nil {
		return targetHost == ruleHost
	}
	return globRuler.Match(targetHost)
}

type trieNode struct {
	children   map[rune]*trieNode
	failure    *trieNode
	patternLen int
	id         int
	flag       int // 对节点的标记，可以用来标记结束节点
}

// IndexAllSubstrings 只遍历一次查找所有子串位置 返回值是一个二维数组，每个元素是一个[2]int类型匹配结果，其中第一个元素是规则index，第二个元素是索引位置
func IndexAllSubstrings(s string, patterns ...string) (result [][2]int) {
	// 构建trie树
	root := &trieNode{
		children:   make(map[rune]*trieNode),
		failure:    nil,
		flag:       0,
		patternLen: 0,
	}

	for patternIndex, pattern := range patterns {
		node := root
		for _, char := range pattern {
			if _, ok := node.children[char]; !ok {
				node.children[char] = &trieNode{
					children:   make(map[rune]*trieNode),
					failure:    nil,
					flag:       0,
					patternLen: 0,
				}
			}
			node = node.children[char]
		}
		node.flag = 1
		node.id = patternIndex
		node.patternLen = len(pattern)
	}
	// 构建Failure
	queue := make([]*trieNode, 0)
	root.failure = root

	for _, child := range root.children {
		child.failure = root
		queue = append(queue, child)
	}

	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:]
		for char, child := range node.children {
			queue = append(queue, child)
			failure := node.failure

			for failure != root && failure.children[char] == nil {
				failure = failure.failure
			}
			if next := failure.children[char]; next != nil {
				child.failure = next
				child.flag = child.flag | next.flag
			} else {
				child.failure = root
			}
		}
	}

	// 查找
	node := root
	for i, char := range s {
		for node != root && node.children[char] == nil {
			node = node.failure
		}

		if next := node.children[char]; next != nil {
			node = next
			if node.flag == 1 {
				result = append(result, [2]int{node.id, i - node.patternLen + 1})
			}
		}
	}
	return
}

func CreateTempTestDatabaseInMemory() (*gorm.DB, error) {
	uuid := uuid.New().String()
	db, err := gorm.Open("sqlite3", "file::memory-"+uuid+"?mode=memory&cache=shared")
	if err != nil {
		return nil, err
	}
	return db, nil
}
