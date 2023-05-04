package yakgrpc

import (
	"fmt"
	"github.com/pkg/errors"
	log "github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"io"
	"io/ioutil"
	"os"
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
