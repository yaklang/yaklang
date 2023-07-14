package vulinbox

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	grpcMetadata "google.golang.org/grpc/metadata"
	"io"
	"net/http"
	"net/url"
)

type VirtualYakExecServer struct {
	send func(result *ypb.ExecResult) error
}

func (v *VirtualYakExecServer) Send(result *ypb.ExecResult) error {
	if v.send == nil {
		panic("not set sender")
	}
	return v.send(result)
}

func (v *VirtualYakExecServer) SetHeader(md grpcMetadata.MD) error {
	//TODO implement me
	panic("implement me")
}

func (v *VirtualYakExecServer) SendHeader(md grpcMetadata.MD) error {
	//TODO implement me
	panic("implement me")
}

func (v *VirtualYakExecServer) SetTrailer(md grpcMetadata.MD) {
	//TODO implement me
	panic("implement me")
}

func (v *VirtualYakExecServer) Context() context.Context {
	return context.Background()
}

func (v *VirtualYakExecServer) SendMsg(m interface{}) error {
	//TODO implement me
	panic("implement me")
}

func (v *VirtualYakExecServer) RecvMsg(m interface{}) error {
	//TODO implement me
	panic("implement me")
}

func NewVirtualYakExecServerWithMessageHandle(h func(result *ypb.ExecResult) error) *VirtualYakExecServer {
	return &VirtualYakExecServer{send: h}
}

func LoadFromGetParams(req *http.Request, name string) string {
	if req == nil {
		return ""
	}
	return req.URL.Query().Get(name)
}

func LoadFromPostParams(req *http.Request, name string) string {
	if req == nil {
		return ""
	}
	raw, err := io.ReadAll(req.Body)
	if err != nil {
		return ""
	}
	vals, err := url.ParseQuery(string(raw))
	if err != nil {
		return ""
	}
	return vals.Get(name)
}

func LoadFromGetJSONParam(req *http.Request, paramsContainer, name string) string {
	var jsonRaw = LoadFromGetParams(req, paramsContainer)
	if jsonRaw, ok := utils.IsJSON(jsonRaw); ok {
		var i = make(map[string]any)
		err := json.Unmarshal([]byte(jsonRaw), &i)
		if err != nil {
			return ""
		}
		return utils.MapGetString(i, name)
	}
	return ""
}

func LoadFromPostJSONParams(req *http.Request, paramsContainer, name string) string {
	var jsonRaw = LoadFromPostParams(req, paramsContainer)
	if jsonRaw, ok := utils.IsJSON(jsonRaw); ok {
		var i = make(map[string]any)
		err := json.Unmarshal([]byte(jsonRaw), &i)
		if err != nil {
			return ""
		}
		return utils.MapGetString(i, name)
	}
	return ""
}

func LoadFromGetBase64Params(req *http.Request, name string) string {
	var raw = LoadFromGetParams(req, name)
	rawBytes, err := codec.DecodeBase64(raw)
	if err != nil {
		return ""
	}
	return string(rawBytes)
}

func LoadFromPostBase64Params(req *http.Request, name string) string {
	var raw = LoadFromPostParams(req, name)
	rawBytes, err := codec.DecodeBase64(raw)
	if err != nil {
		return ""
	}
	return string(rawBytes)
}

func LoadFromGetBase64JSONParam(req *http.Request, containerName, name string) string {
	var jsonRaw = LoadFromGetBase64Params(req, containerName)
	if jsonRaw, ok := utils.IsJSON(jsonRaw); ok {
		var i = make(map[string]any)
		err := json.Unmarshal([]byte(jsonRaw), &i)
		if err != nil {
			return ""
		}
		return utils.MapGetString(i, name)
	}
	return ""
}

func LoadFromPostBase64JSONParams(req *http.Request, container, name string) string {
	var jsonRaw = LoadFromPostBase64Params(req, container)
	if jsonRaw, ok := utils.IsJSON(jsonRaw); ok {
		var i = make(map[string]any)
		err := json.Unmarshal([]byte(jsonRaw), &i)
		if err != nil {
			return ""
		}
		return utils.MapGetString(i, name)
	}
	return ""
}

func LoadFromBodyJsonParams(req *http.Request, name string) string {
	raw, err := utils.HttpDumpWithBody(req, true)
	if err != nil {
		return ""
	}
	if raw, ok := utils.IsJSON(string(raw)); ok {
		var i = make(map[string]any)
		err := json.Unmarshal([]byte(raw), &i)
		if err != nil {
			return ""
		}
		return utils.MapGetString(i, name)
	}
	return ""
}

func Failed(writer http.ResponseWriter, r *http.Request, msg string, items ...any) {
	if items != nil {
		msg = fmt.Sprintf(msg, items...)
	}
	writer.Header().Set("Content-Type", "text/plain")
	var raw, _ = utils.HttpDumpWithBody(r, true)
	writer.Write(raw)
	writer.Write([]byte{'\r', '\n', '\r', '\n', '\r', '\n'})
	writer.Write([]byte("-----------------------------------------------------\n Failed: "))
	_, _ = writer.Write([]byte(msg))
	writer.WriteHeader(500)
}
