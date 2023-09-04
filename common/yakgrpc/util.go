package yakgrpc

import (
	"bufio"
	"bytes"
	"fmt"
	"github.com/pkg/errors"
	log "github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"gopkg.in/yaml.v2"
	"io"
	"io/ioutil"
	"os"
	"reflect"
	"strings"
)

// OpenPortServerStreamerHelperRWC
type OpenPortServerStreamerHelperRWC struct {
	io.ReadWriteCloser

	stream     ypb.Yak_OpenPortServer
	rbuf       []byte
	LocalAddr  string
	RemoveAddr string
}

func (c *OpenPortServerStreamerHelperRWC) Read(b []byte) (n int, _ error) {
	if len(c.rbuf) > 0 {
		n = copy(b, c.rbuf)
		c.rbuf = c.rbuf[n:]
		return n, nil
	}

	msg, err := c.stream.Recv()
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

type YamlMapBuilder yaml.MapSlice
type YamlArrayBuilder struct {
	slice *[]*yaml.MapSlice
}

func (a *YamlArrayBuilder) Add(slice *YamlMapBuilder) {
	*a.slice = append(*a.slice, (*yaml.MapSlice)(slice))
}
func NewYamlMapBuilder() *YamlMapBuilder {
	return &YamlMapBuilder{}
}
func (m *YamlMapBuilder) FilterEmptyField() *yaml.MapSlice {
	var res yaml.MapSlice
	for _, item := range *m {
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
				(*ret)[i] = (*YamlMapBuilder)(slice).FilterEmptyField()
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
func (m *YamlMapBuilder) Set(k string, v any) {
	*m = append(*m, yaml.MapItem{
		Key:   k,
		Value: v,
	})
}
func (m *YamlMapBuilder) AddEmptyLine() {
	m.Set("__empty_line__", "__empty_line__")
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
		if strings.Contains(line, "__empty_line__: ") {
			line = ""
		}
		res += line + "\n"
	}
	return res, err
}
