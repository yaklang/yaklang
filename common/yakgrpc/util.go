package yakgrpc

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"os"
	"path"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/thirdparty_bin"
	"github.com/yaklang/yaklang/common/yak/yaklang"

	"github.com/jinzhu/gorm"
	"github.com/pkg/errors"
	"github.com/segmentio/ksuid"
	log "github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/netx"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"google.golang.org/grpc"
	"gopkg.in/yaml.v2"
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
	initLocalClientOnce sync.Once
	ciClient            ypb.YakClient
	ciClientOnce        sync.Once
)

type Client struct {
	ypb.YakClient
	server *Server
}

func (c *Client) GetProfileDatabase() *gorm.DB {
	if c.server == nil {
		return nil
	}
	return c.server.profileDatabase
}

func (c *Client) GetProjectDatabase() *gorm.DB {
	if c.server == nil {
		return nil
	}
	return c.server.projectDatabase
}

func NewLocalClient(locals ...bool) (ypb.YakClient, error) {
	local := false
	if len(locals) > 0 {
		local = locals[0]
	}
	return newLocalClientEx(local)
}

func NewLocalClientForceNew() (ypb.YakClient, error) {
	port := utils.GetRandomAvailableTCPPort()
	addr := utils.HostPort("127.0.0.1", port)
	grpcTrans := grpc.NewServer(
		grpc.MaxRecvMsgSize(100*1024*1024),
		grpc.MaxSendMsgSize(100*1024*1024),
	)
	opts := []ServerOpts{WithInitFacadeServer(true)}
	var (
		profileDatabasePath, projectDatabasePath string
	)
	s, err := newServerEx(opts...)
	if err != nil {
		log.Errorf("build yakit server failed: %s", err)
		return nil, err
	}
	ypb.RegisterYakServer(grpcTrans, s)
	var lis net.Listener
	lis, err = net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}
	go func() {
		defer func() {
			if profileDatabasePath != "" {
				os.Remove(profileDatabasePath)
			}
			if projectDatabasePath != "" {
				os.Remove(projectDatabasePath)
			}
		}()
		err = grpcTrans.Serve(lis)
		if err != nil {
			log.Error(err)
		}
	}()
	time.Sleep(1 * time.Second)
	conn, err := grpc.Dial(addr, grpc.WithInsecure(), grpc.WithDefaultCallOptions(
		grpc.MaxCallRecvMsgSize(100*1024*1045),
		grpc.MaxCallRecvMsgSize(100*1024*1045),
	))
	return &Client{
		YakClient: ypb.NewYakClient(conn),
		server:    s,
	}, err
}

func NewLocalClientWithTempDatabase(t *testing.T) (ypb.YakClient, error) {
	var port int
	var addr string
	netx.UnsetProxyFromEnv()

	port = utils.GetRandomAvailableTCPPort()
	addr = utils.HostPort("127.0.0.1", port)
	grpcTrans := grpc.NewServer(
		grpc.MaxRecvMsgSize(100*1024*1024),
		grpc.MaxSendMsgSize(100*1024*1024),
	)
	profileDatabasePath := path.Join(os.TempDir(), fmt.Sprintf("%s.db", ksuid.New().String()))
	projectDatabasePath := path.Join(os.TempDir(), fmt.Sprintf("%s.db", ksuid.New().String()))
	s, err := newServerEx(WithInitFacadeServer(true), WithProfileDatabasePath(profileDatabasePath), WithProjectDatabasePath(projectDatabasePath))
	if err != nil {
		log.Errorf("build yakit server failed: %s", err)
		return nil, err
	}
	ypb.RegisterYakServer(grpcTrans, s)
	var lis net.Listener
	lis, err = net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}
	t.Cleanup(func() {
		os.Remove(profileDatabasePath)
		os.Remove(projectDatabasePath)
	})
	go func() {
		defer func() {

		}()
		err = grpcTrans.Serve(lis)
		if err != nil {
			log.Error(err)
		}
	}()
	time.Sleep(1 * time.Second)
	conn, err := grpc.Dial(addr, grpc.WithInsecure(), grpc.WithDefaultCallOptions(
		grpc.MaxCallRecvMsgSize(100*1024*1045),
		grpc.MaxCallRecvMsgSize(100*1024*1045),
	))
	client := &Client{
		YakClient: ypb.NewYakClient(conn),
		server:    s,
	}
	return client, err
}

func newLocalClientEx(local bool) (ypb.YakClient, error) {
	var port int
	var addr string
	netx.UnsetProxyFromEnv()

	dialServer := func(addr string, server *Server) (ypb.YakClient, error) {
		conn, err := grpc.Dial(addr, grpc.WithInsecure(), grpc.WithDefaultCallOptions(
			grpc.MaxCallRecvMsgSize(100*1024*1045),
			grpc.MaxCallRecvMsgSize(100*1024*1045),
		))
		return &Client{
			YakClient: ypb.NewYakClient(conn),
			server:    server,
		}, err
	}

	if local || !utils.InGithubActions() {
		var finalErr error
		initLocalClientOnce.Do(func() {
			yaklang.Import("test", map[string]any{
				"callhook": func(name string) any {
					return callHook(name)
				},
			})
			port = utils.GetRandomAvailableTCPPort()
			addr = utils.HostPort("127.0.0.1", port)
			grpcTrans := grpc.NewServer(
				grpc.MaxRecvMsgSize(100*1024*1024),
				grpc.MaxSendMsgSize(100*1024*1024),
			)
			opts := []ServerOpts{WithInitFacadeServer(true)}
			var (
				profileDatabasePath, projectDatabasePath string
			)
			s, err := newServerEx(opts...)
			if err != nil {
				log.Errorf("build yakit server failed: %s", err)
				finalErr = err
				return
			}
			ypb.RegisterYakServer(grpcTrans, s)
			var lis net.Listener
			lis, err = net.Listen("tcp", addr)
			if err != nil {
				finalErr = err
				return
			}
			go func() {
				defer func() {
					if profileDatabasePath != "" {
						os.Remove(profileDatabasePath)
					}
					if projectDatabasePath != "" {
						os.Remove(projectDatabasePath)
					}
				}()
				err = grpcTrans.Serve(lis)
				if err != nil {
					log.Error(err)
				}
			}()
			time.Sleep(1 * time.Second)
			localClient, finalErr = dialServer(addr, s)
		})
		if finalErr != nil {
			return nil, finalErr
		}
		return localClient, nil
	} else {
		addr = utils.HostPort("127.0.0.1", 8087)
		var finalErr error
		ciClientOnce.Do(func() {
			ciClient, finalErr = dialServer(addr, nil)
		})
		return ciClient, finalErr
	}
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
