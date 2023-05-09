package yakgrpc

import (
	"context"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/wsm"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func (s *Server) CreateWebShell(ctx context.Context, req *ypb.WebShell) (*ypb.Empty, error) {
	db := consts.GetGormProjectDatabase()
	if db == nil {
		log.Error("empty database")
		return nil, utils.Errorf("no database connection")
	}
	shell := &yakit.WebShell{
		Url:           req.GetUrl(),
		Pass:          req.GetPass(),
		SecretKey:     req.GetSecretKey(),
		EncryptedMode: req.GetEncMode(),
		Charset:       req.GetCharset(),
		ShellType:     req.GetShellType(),
		ShellScript:   req.GetShellScript(),
		Tag:           req.GetTag(),
	}
	err := yakit.CreateOrUpdateWebShell(db, shell.CalcHash(), shell)
	if err != nil {
		log.Error(err)
		return nil, err
	}
	return &ypb.Empty{}, nil
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

func (s *Server) UpdateWebShellById(ctx context.Context, req *ypb.UpdateWebShellRequest) (*ypb.Empty, error) {
	db := consts.GetGormProjectDatabase()
	if db == nil {
		log.Error("empty database")
		return nil, utils.Errorf("no database connection")
	}
	shell := &yakit.WebShell{
		Url:           req.GetUrl(),
		Pass:          req.GetPass(),
		SecretKey:     req.GetSecretKey(),
		EncryptedMode: req.GetEncMode(),
		Charset:       req.GetCharset(),
		ShellType:     req.GetShellType(),
		ShellScript:   req.GetShellScript(),
		Tag:           req.GetTag(),
	}
	err := yakit.CreateOrUpdateWebShellById(db, req.GetId(), shell)
	if err != nil {
		log.Error(err)
		return nil, err
	}
	return &ypb.Empty{}, nil
}

func (s *Server) QueryWebShells(ctx context.Context, req *ypb.QueryWebShellsRequest) (*ypb.QueryWebShellsResponse, error) {
	db := consts.GetGormProjectDatabase()
	if db == nil {
		log.Error("empty database")
		return nil, utils.Errorf("no database connection")
	}
	p, res, err := yakit.QueryWebShell(db, req)
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
	shell, err := yakit.GetWebShell(db, req.GetId())
	if err != nil {
		return nil, err
	}
	w, err := wsm.NewWsm(shell)
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
	ping, err := w.Ping()
	shell.State = ping
	if err != nil {
		yakit.CreateOrUpdateWebShellById(db, req.GetId(), shell)
		return nil, err
	}

	err = yakit.CreateOrUpdateWebShellById(db, req.GetId(), shell)
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
	w, err := wsm.NewWsm(shell)
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
