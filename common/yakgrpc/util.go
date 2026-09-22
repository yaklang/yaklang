package yakgrpc

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"os"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/thirdparty_bin"
	"github.com/yaklang/yaklang/common/yak/yaklang"

	"github.com/pkg/errors"
	"github.com/yaklang/gorm"
	log "github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/netx"
	"github.com/yaklang/yaklang/common/utils"
	yaml "github.com/yaklang/yaklang/common/utils/orderedyaml"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/test/bufconn"
)

const (
	DbOperationCreate         = "create"
	DbOperationDelete         = "delete"
	DbOperationUpdate         = "update"
	DbOperationQuery          = "query"
	DbOperationCreateOrUpdate = "create_or_update"
)

var CallHookMap = sync.Map{}

func callHook(name string) any {
	v, ok := CallHookMap.Load(name)
	if ok {
		if f, ok := v.(func() any); ok {
			return f()
		}
	}
	return nil
}

// OpenPortServerStreamerHelperRWC
type OpenPortServerStreamerHelperRWC struct {
	io.ReadWriteCloser

	stream       ypb.Yak_OpenPortServer
	rbuf         []byte
	LocalAddr    string
	RemoveAddr   string
	sizeCallback func(width, height int)
}

func (c *OpenPortServerStreamerHelperRWC) Read(b []byte) (n int, _ error) {
	if len(c.rbuf) > 0 {
		n = copy(b, c.rbuf)
		c.rbuf = c.rbuf[n:]
		return n, nil
	}

	msg, err := c.stream.Recv()
	// control message
	if c.sizeCallback != nil && msg.GetWidth() > 0 {
		c.sizeCallback(int(msg.GetWidth()), int(msg.GetHeight()))
		return 0, nil
	}
	if err != nil {
		return 0, errors.Errorf("failed to recv from client stream: %s", err)
	}

	n = copy(b, msg.GetRaw())
	c.rbuf = msg.GetRaw()[n:]
	return n, nil
}

func (s *OpenPortServerStreamerHelperRWC) Write(b []byte) (int, error) {
	log.Debugf("send[%d]: %s", len(b), string(b))
	err := s.stream.Send(&ypb.Output{
		Raw:        b,
		RemoteAddr: s.RemoveAddr,
		LocalAddr:  s.LocalAddr,
	})
	if err != nil {
		return 0, err
	}
	return len(b), err
}

func (s *OpenPortServerStreamerHelperRWC) Close() (err error) {
	return nil
}

// ----------------------------------------------------------------------------------------

// OpenPortServerStreamerHelperRWC
type YakOutputStreamerHelperWC struct {
	io.WriteCloser

	stream ypb.Yak_ExecServer
	rbuf   []byte
}

func (s *YakOutputStreamerHelperWC) Write(b []byte) (int, error) {
	log.Debugf("send[%d]: %s", len(b), string(b))
	err := s.stream.Send(&ypb.ExecResult{
		Raw: b,
	})
	if err != nil {
		return 0, err
	}
	return len(b), err
}

func (s *YakOutputStreamerHelperWC) Close() (err error) {
	return nil
}

// ----------------------------------------------------------------------------------------

/*
一键处理 pluginNames 作为参数
*/
func appendPluginNames(params []*ypb.ExecParamItem, plugins ...string) ([]*ypb.ExecParamItem, func(), error) {
	return appendPluginNamesEx("yakit-plugin-file", "|", params, plugins...)
}

func KVPairToParamItem(params []*ypb.KVPair) []*ypb.ExecParamItem {
	res := []*ypb.ExecParamItem{}
	for _, i := range params {
		res = append(res, &ypb.ExecParamItem{Key: i.Key, Value: i.Value})
	}
	return res
}

func ParamItemToKVPair(params []*ypb.ExecParamItem) []*ypb.KVPair {
	res := []*ypb.KVPair{}
	for _, i := range params {
		res = append(res, &ypb.KVPair{Key: i.Key, Value: i.Value})
	}
	return res
}

func appendPluginNamesExKVPair(key string, splitStr string, params []*ypb.KVPair, plugins ...string) ([]*ypb.KVPair, func(), error) {
	item, f, err := appendPluginNamesEx(key, splitStr, KVPairToParamItem(params), plugins...)
	if err != nil {
		return nil, nil, err
	}
	return ParamItemToKVPair(item), f, nil
}

func appendPluginNamesEx(key string, splitStr string, params []*ypb.ExecParamItem, plugins ...string) ([]*ypb.ExecParamItem, func(), error) {
	// handle plugin names
	names := plugins
	callback := func() {}
	if names != nil {
		fp, err := ioutil.TempFile("", "yakit-scan-port-plugins-*.txt")
		if err != nil {
			msg := fmt.Sprintf("create yakit-scan-port-plugins list failed: %s", err)
			log.Error(msg)
			return params, callback, utils.Error(msg)
		}

		if fp != nil {
			callback = func() {
				os.RemoveAll(fp.Name())
			}
			for _, i := range plugins {
				fp.WriteString(i + splitStr)
			}
			fp.Close()
			log.Infof("use plugin list in %v", fp.Name())
			params = append(params, &ypb.ExecParamItem{Key: key, Value: fp.Name()})
		}
	} else {
		log.Info("loading plugin empty")
	}
	return params, callback, nil
}

var (
	localClient         ypb.YakClient
	localClientInitErr  error
	initLocalClientOnce sync.Once
)

type Client struct {
	ypb.YakClient
	server    *Server
	closeOnce sync.Once
	closeFunc func() error
}

func (c *Client) GetProfileDatabase() *gorm.DB {
	// Delegate to Server.GetProfileDatabase() which falls back to the
	// consts global singleton when Server.profileDatabase is nil (the
	// common production case – the field is only set for test servers that
	// use WithProfileDatabasePath). Direct field access here would return
	// nil in production and cause panics in callers (e.g. MCP payload handlers).
	if c.server == nil {
		return nil
	}
	return c.server.GetProfileDatabase()
}

func (c *Client) GetProjectDatabase() *gorm.DB {
	// Same reasoning as GetProfileDatabase above.
	if c.server == nil {
		return nil
	}
	return c.server.GetProjectDatabase()
}

// NewLocalClient returns the process-wide in-memory gRPC client. The optional
// legacy argument is retained for source compatibility; CI uses the same path.
func NewLocalClient(_ ...bool) (ypb.YakClient, error) {
	initLocalClientOnce.Do(func() {
		netx.UnsetProxyFromEnv()
		yaklang.Import("test", map[string]any{"callhook": func(name string) any { return callHook(name) }})
		localClient, localClientInitErr = NewLocalClientForceNew()
	})
	return localClient, localClientInitErr
}

// Close releases an independently created local client's transport. Do not close
// the shared client returned by NewLocalClient while other callers are using it.
func (c *Client) Close() error {
	var err error
	c.closeOnce.Do(func() {
		if c.closeFunc != nil {
			err = c.closeFunc()
		}
	})
	return err
}

func NewLocalClientForceNew() (ypb.YakClient, error) {
	s, err := newServerEx(WithInitFacadeServer(false))
	if err != nil {
		return nil, err
	}
	return newInMemoryClient(s)
}

// newInMemoryClient preserves protobuf serialization, interceptors, deadlines,
// and streaming semantics without binding a TCP port or sleeping for readiness.
func newInMemoryClient(s *Server) (*Client, error) {
	lis := bufconn.Listen(1024 * 1024)
	transport := grpc.NewServer(grpc.MaxRecvMsgSize(100*1024*1024), grpc.MaxSendMsgSize(100*1024*1024))
	ypb.RegisterYakServer(transport, s)
	done := make(chan struct{})
	go func() { defer close(done); _ = transport.Serve(lis) }()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	conn, err := grpc.DialContext(ctx, "passthrough:///yak-local",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return lis.DialContext(ctx) }),
		grpc.WithInsecure(), grpc.WithBlock(),
		grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(100*1024*1024), grpc.MaxCallSendMsgSize(100*1024*1024)))
	stop := func() { transport.Stop(); _ = lis.Close(); <-done }
	if err != nil {
		stop()
		return nil, err
	}
	return &Client{YakClient: ypb.NewYakClient(conn), server: s, closeFunc: func() error {
		err := conn.Close()
		stop()
		return err
	}}, nil
}

type YamlMapBuilder struct {
	keySet         map[string]struct{} // 去重，如果存在多个相同的key，只保留第一个
	forceKeySet    map[string]struct{}
	slice          *yaml.MapSlice
	defaultField   map[string]any // field的默认值，如果新增字段是默认值，则跳过
	emptyLineIndex int
}
type YamlArrayBuilder struct {
	slice *[]*yaml.MapSlice
}

func (a *YamlMapBuilder) SetDefaultField(fieldMap map[string]any) {
	a.defaultField = fieldMap
}

func (a *YamlArrayBuilder) Add(slice *YamlMapBuilder) {
	*a.slice = append(*a.slice, slice.slice)
}

func NewYamlMapBuilder() *YamlMapBuilder {
	return &YamlMapBuilder{
		keySet:       make(map[string]struct{}),
		defaultField: make(map[string]any),
		slice:        &yaml.MapSlice{},
		forceKeySet:  make(map[string]struct{}),
	}
}

func (m *YamlMapBuilder) FilterEmptyField() *yaml.MapSlice {
	var res yaml.MapSlice
	for _, item := range *m.slice {
		if _, ok := m.forceKeySet[utils.InterfaceToString(item.Key)]; ok {
			res = append(res, item)
			continue
		}
		isRawHTTP := item.Key == "raw"

		// 特殊处理：字符串数组中包含多行字符串，标记为需要 literal 处理
		if strArray, ok := item.Value.([]string); ok && isRawHTTP && len(strArray) > 0 {
			// 使用特殊前缀标记，类似 __comment__ 的处理方式
			// 将数组编码为特殊格式：__literal_array__:hexencoded
			var encoded []string
			for _, str := range strArray {
				encoded = append(encoded, codec.EncodeToHex(str))
			}
			// 用特殊格式存储：使用单个字符串，用 | 分隔各个元素的 hex
			item.Value = "__literal_array__:" + strings.Join(encoded, "|")
		}

		switch ret := item.Value.(type) {
		case *YamlMapBuilder:
			item.Value = ret.FilterEmptyField()
		case string:
			if ret == "" {
				continue
			}
		case *[]*yaml.MapSlice:
			if len(*ret) == 0 {
				continue
			}
			for i, slice := range *ret {
				(*ret)[i] = (&YamlMapBuilder{slice: slice}).FilterEmptyField()
			}
		}
		if reflect.TypeOf(item.Value).Kind() == reflect.Array || reflect.TypeOf(item.Value).Kind() == reflect.Slice {
			if reflect.ValueOf(item.Value).Len() == 0 {
				continue
			}
		}
		if reflect.TypeOf(item.Value).Kind() == reflect.Ptr && (reflect.ValueOf(item.Value).IsNil() || reflect.ValueOf(item.Value).Elem().IsNil()) {
			continue
		}
		res = append(res, item)
	}
	return &res
}

func (m *YamlMapBuilder) ForceSet(k string, v any) {
	if _, ok := m.keySet[k]; ok {
		return
	}
	m.keySet[k] = struct{}{}
	m.forceKeySet[k] = struct{}{}
	*m.slice = append(*m.slice, yaml.MapItem{
		Key:   k,
		Value: v,
	})
}

func (m *YamlMapBuilder) Set(k string, v any) {
	if _, ok := m.keySet[k]; ok {
		return
	}
	if m.defaultField != nil {
		if v2, ok := m.defaultField[k]; ok {
			if v == v2 {
				return
			}
		}
	}
	m.keySet[k] = struct{}{}
	*m.slice = append(*m.slice, yaml.MapItem{
		Key:   k,
		Value: v,
	})
}

func (m *YamlMapBuilder) AddEmptyLine() {
	m.emptyLineIndex++
	m.Set("__empty_line__"+strconv.Itoa(m.emptyLineIndex), "__empty_line__")
}

func (m *YamlMapBuilder) AddComment(comment string) {
	m.Set("__comment__", codec.EncodeToHex(comment))
}

func (m *YamlMapBuilder) NewSubMapBuilder(k string) *YamlMapBuilder {
	newSliceUtil := NewYamlMapBuilder()
	m.Set(k, newSliceUtil)
	return newSliceUtil
}

func (m *YamlMapBuilder) NewSubArrayBuilder(k string) *YamlArrayBuilder {
	var v []*yaml.MapSlice
	m.Set(k, &v)
	return &YamlArrayBuilder{slice: &v}
}

func (m *YamlMapBuilder) MarshalToString() (string, error) {
	var res string
	yamlContent, err := yaml.Marshal(m.FilterEmptyField())
	scanner := bufio.NewScanner(bytes.NewReader(yamlContent))
	scanner.Split(bufio.ScanLines)
	for scanner.Scan() {
		line := scanner.Text()
		if i := strings.Index(line, "__comment__:"); i != -1 {
			padding := strings.Repeat(" ", i)
			hexComment := strings.TrimSpace(line[i+len("__comment__:"):])
			comment, err := codec.DecodeHex(hexComment)
			if err != nil {
				log.Errorf("decode hex comment failed: %s", err)
				continue
			}
			commentLines := strings.Split(string(comment), "\n")
			for _, commentLine := range commentLines {
				res += padding + "# " + commentLine + "\n"
			}
			continue
		}

		// 处理 literal array 标记（新增逻辑，参考 __comment__ 的处理方式）
		if strings.Contains(line, "__literal_array__:") {
			// 提取字段名和编码的数组内容
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				fieldPart := parts[0]
				encodedPart := strings.TrimSpace(parts[1])

				encodedPart = strings.Trim(encodedPart, "\"")
				if strings.HasPrefix(encodedPart, "__literal_array__:") {
					padding := strings.Repeat(" ", len(fieldPart)-len(strings.TrimLeft(fieldPart, " ")))
					fieldName := strings.TrimSpace(fieldPart)

					// 输出字段名
					res += padding + fieldName + ":\n"

					// 解码数组内容
					encodedArray := strings.TrimPrefix(encodedPart, "__literal_array__:")
					hexParts := strings.Split(encodedArray, "|")

					// 输出为 literal block scalar 格式
					for i, hexStr := range hexParts {
						decoded, err := codec.DecodeHex(hexStr)
						if err != nil {
							log.Errorf("decode hex literal array failed: %s", err)
							continue
						}

						// 输出 literal block scalar: |
						res += padding + "  - |\n"
						contentLines := strings.Split(string(decoded), "\n")
						// 移除末尾的空字符串（由末尾换行符产生）
						// 因为 | 格式会自动在末尾添加一个换行
						for len(contentLines) > 0 && contentLines[len(contentLines)-1] == "" {
							contentLines = contentLines[:len(contentLines)-1]
						}
						for _, contentLine := range contentLines {
							res += padding + "    " + contentLine + "\n"
						}

						// 在每个请求之间添加一个空行（最后一个除外），符合 Nuclei 官方格式
						if i < len(hexParts)-1 {
							res += "\n"
						}
					}
					continue
				}
			}
		}

		if strings.Contains(line, "__empty_line__") {
			line = ""
		}
		res += line + "\n"
	}
	return res, err
}

func NewGrpcProgressCallback(stream ypb.Yak_InstallThirdPartyBinaryServer) thirdparty_bin.ProgressCallback {
	return func(progress float64, downloaded, total int64, message string) {
		stream.Send(&ypb.ExecResult{
			IsMessage: true,
			Message:   []byte(message),
			Progress:  float32(progress * 100),
		})
	}
}
