package yakgrpc

import (
	"context"
	"encoding/json"
	"errors"

	pta "github.com/yaklang/yaklang/common/yak/plugin_type_analyzer"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func yaklangInfo2Grpc(infos []*pta.YaklangInfo) []*ypb.YaklangInformation {
	ret := make([]*ypb.YaklangInformation, 0, len(infos))

	var infoKV2grpc func(kvs []*pta.YaklangInfoKV) []*ypb.YaklangInformationKV
	infoKV2grpc = func(kvs []*pta.YaklangInfoKV) []*ypb.YaklangInformationKV {
		ret := make([]*ypb.YaklangInformationKV, 0, len(kvs))
		for _, kv := range kvs {
			data, err := json.Marshal(kv.Value)
			if err != nil {
				continue
			}
			ret = append(ret, &ypb.YaklangInformationKV{
				Key:    kv.Key,
				Value:  data,
				Extern: infoKV2grpc(kv.Extern),
			})
		}
		return ret
	}

	for _, info := range infos {
		ret = append(ret, &ypb.YaklangInformation{
			Name: info.Name,
			Data: infoKV2grpc(info.KV),
		})
	}
	return ret
}


func (s *Server) YaklangInspectInformation(ctx context.Context, req *ypb.YaklangInspectInformationRequest) (*ypb.YaklangInspectInformationResponse, error) {
	ret := &ypb.YaklangInspectInformationResponse{}
	prog := ssaapi.Parse(req.YakScriptCode, pta.GetPluginSSAOpt(req.YakScriptType)...)
	if prog.IsNil() {
		return nil, errors.New("ssa parse error")
	}

	ret.Information = yaklangInfo2Grpc(pta.GetPluginInfo(req.YakScriptType, prog))

	return ret, nil
}
