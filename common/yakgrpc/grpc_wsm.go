package yakgrpc

import (
	"context"
	"encoding/json"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/wsm"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"strings"
)

func (s *Server) CreateWebShell(ctx context.Context, req *ypb.WebShell) (*ypb.WebShell, error) {
	db := consts.GetGormProjectDatabase()
	if db == nil {
		log.Error("empty database")
		return nil, utils.Errorf("no database connection")
	}
	var headers string
	if req.GetHeaders() != nil {
		b, err := json.Marshal(req.GetHeaders())
		if err != nil {
			return nil, utils.Errorf("headers marshal error: %v", err)
		}
		headers = string(b)
	}

	shell := &yakit.WebShell{
		Url:              req.GetUrl(),
		Pass:             req.GetPass(),
		SecretKey:        req.GetSecretKey(),
		EncryptedMode:    req.GetEncMode(),
		Charset:          req.GetCharset(),
		ShellType:        req.GetShellType(),
		ShellScript:      req.GetShellScript(),
		Headers:          headers,
		Tag:              req.GetTag(),
		Proxy:            req.GetProxy(),
		Remark:           req.GetRemark(),
		PayloadCodecName: req.GetPayloadCodecName(),
		PacketCodecName:  req.GetPacketCodecName(),
	}
	webShell, err := yakit.CreateOrUpdateWebShell(db, shell.CalcHash(), shell)
	if err != nil {
		return nil, utils.Errorf("create webshell error: %v", err)
	}
	return webShell.ToGRPCModel(), nil
}

func (s *Server) DeleteWebShell(ctx context.Context, req *ypb.DeleteWebShellRequest) (*ypb.Empty, error) {
	db := consts.GetGormProjectDatabase()
	if db == nil {
		log.Error("empty database")
		return nil, utils.Errorf("no database connection")
	}
	if len(req.GetIds()) > 0 {
		for _, i := range req.GetIds() {
			_ = yakit.DeleteWebShellByID(db, i)
		}
		return &ypb.Empty{}, nil
	}
	if req.Id > 0 {
		_ = yakit.DeleteWebShellByID(db, req.Id)
		return &ypb.Empty{}, nil
	}
	return &ypb.Empty{}, nil
}

func (s *Server) UpdateWebShell(ctx context.Context, req *ypb.WebShell) (*ypb.WebShell, error) {
	db := consts.GetGormProjectDatabase()
	if db == nil {
		log.Error("empty database")
		return nil, utils.Errorf("no database connection")
	}
	var headers string
	if req.GetHeaders() != nil {
		b, err := json.Marshal(req.GetHeaders())
		if err != nil {
			return nil, utils.Errorf("headers marshal error: %v", err)
		}
		headers = string(b)
	}

	shellMap := map[string]interface{}{
		"url":                req.GetUrl(),
		"pass":               req.GetPass(),
		"secret_key":         req.GetSecretKey(),
		"enc_mode":           req.GetEncMode(),
		"charset":            req.GetCharset(),
		"shell_type":         req.GetShellType(),
		"shell_script":       req.GetShellScript(),
		"headers":            headers,
		"status":             req.GetStatus(),
		"tag":                req.GetTag(),
		"proxy":              req.GetProxy(),
		"remark":             req.GetRemark(),
		"payload_codec_name": req.GetPayloadCodecName(),
		"packet_codec_name":  req.GetPacketCodecName(),
	}
	webShell, err := yakit.UpdateWebShellById(db, req.GetId(), shellMap)
	if err != nil {
		log.Error(err)
		return nil, err
	}
	return webShell.ToGRPCModel(), nil
}

func (s *Server) QueryWebShells(ctx context.Context, req *ypb.QueryWebShellsRequest) (*ypb.QueryWebShellsResponse, error) {
	db := consts.GetGormProjectDatabase()
	if db == nil {
		log.Error("empty database")
		return nil, utils.Errorf("no database connection")
	}
	p, res, err := yakit.QueryWebShells(db, req)
	if err != nil {
		return nil, err
	}
	rsp := &ypb.QueryWebShellsResponse{
		Pagination: req.Pagination,
		Total:      int64(p.TotalRecord),
	}
	for _, d := range res {
		rsp.Data = append(rsp.Data, d.ToGRPCModel())
	}
	return rsp, nil
}

func (s *Server) Ping(ctx context.Context, req *ypb.WebShellRequest) (*ypb.WebShellResponse, error) {
	db := consts.GetGormProjectDatabase()
	if db == nil {
		log.Error("empty database")
		return nil, utils.Errorf("no database connection")
	}
	var err error
	shell, err := yakit.GetWebShell(db, req.GetId())
	if err != nil {
		return nil, err
	}
	w, err := wsm.NewWebShellManager(shell)
	if err != nil {
		return nil, err
	}
	if shell.GetPacketCodecName() != "" {
		script, err := yakit.GetYakScriptByName(s.GetProfileDatabase(), shell.GetPacketCodecName())
		if err != nil {
			return nil, err
		}

		w.SetPacketScriptContent(script.Content)
	}
	if shell.GetPayloadCodecName() != "" {
		script, err := yakit.GetYakScriptByName(s.GetProfileDatabase(), shell.GetPayloadCodecName())
		if err != nil {
			return nil, err
		}
		w.SetPayloadScriptContent(script.Content)
	}
	ping, err := w.Ping()
	if err != nil {
		return nil, err
	}
	shell.Status = ping

	_, err = yakit.UpdateWebShellById(db, req.GetId(), shell)
	if err != nil {
		log.Error(err)
		return nil, err
	}
	data := &ypb.WebShellResponse{State: ping}
	return data, nil
}

func (s *Server) GetBasicInfo(ctx context.Context, req *ypb.WebShellRequest) (*ypb.WebShellResponse, error) {
	db := consts.GetGormProjectDatabase()
	if db == nil {
		log.Error("empty database")
		return nil, utils.Errorf("no database connection")
	}
	shell, err := yakit.GetWebShell(db, req.GetId())
	if err != nil {
		return nil, err
	}
	w, err := wsm.NewWebShellManager(shell)
	if err != nil {
		return nil, err
	}
	g, ok := w.(*wsm.Godzilla)
	if ok {
		err := g.InjectPayload()
		if err != nil {
			return nil, err
		}
	}
	info, err := w.BasicInfo()
	if err != nil {
		return nil, err
	}
	return &ypb.WebShellResponse{State: true, Data: info}, nil
}

func getWebShellCodec(name string) (string, string, error) {
	db := consts.GetGormProfileDatabase()
	script, err := yakit.GetYakScriptByName(db, name)
	if err != nil {
		return "", "", err
	}
	contents := strings.Split(script.Content, "##############################################")
	if len(contents) == 2 {
		return contents[0], contents[1], nil
	}
	return "", "", utils.Errorf("invalid packet codec script")
}
