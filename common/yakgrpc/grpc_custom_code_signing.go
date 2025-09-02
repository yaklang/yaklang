package yakgrpc

import (
	"context"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func (s *Server) CreateCustomCode(ctx context.Context, req *ypb.CustomCodeRequest) (*ypb.Empty, error) {
	if err := yakit.CreateCustomCodeSigning(s.GetProjectDatabase(), schema.NewCustomCodeSigning(req)); err != nil {
		return nil, err
	}
	return &ypb.Empty{}, nil
}

func (s *Server) UpdateCustomCode(ctx context.Context, req *ypb.EditCustomCodeRequest) (*ypb.Empty, error) {
	target := req.GetTarget()

	if err := yakit.UpdateCustomCodeSigning(s.GetProjectDatabase(), target, &schema.CustomCodeSigning{
		CustomCodeId:    "",
		CustomCodeName:  req.GetName(),
		CustomCodeData:  req.GetCode(),
		CustomCodeDesc:  req.GetDescription(),
		CustomCodeState: schema.SwitcCustomCodeSigningType(req.GetState()),
		CustomCodeLevel: schema.CustomCodeSigningLevel(req.GetLevel()),
	}); err != nil {
		return nil, err
	}
	return &ypb.Empty{}, nil
}

func (s *Server) DeleteCustomCode(ctx context.Context, req *ypb.QueryCustomCodeRequest) (*ypb.Empty, error) {
	filter := req.GetFilter()

	if err := yakit.DeleteCustomCodeSignings(s.GetProjectDatabase(), filter); err != nil {
		return nil, err
	}
	return &ypb.Empty{}, nil
}

func (s *Server) QueryCustomCode(ctx context.Context, req *ypb.QueryCustomCodeRequest) (*ypb.CustomCodeResponse, error) {
	filter := req.GetFilter()

	CustomCodeSignings, err := yakit.GetCustomCodeSignings(s.GetProjectDatabase(), filter)
	if err != nil {
		return nil, err
	}
	return &ypb.CustomCodeResponse{
		Names: lo.Map(CustomCodeSignings, func(c *schema.CustomCodeSigning, _ int) string {
			return c.CustomCodeName
		}),
		Codes: lo.Map(CustomCodeSignings, func(c *schema.CustomCodeSigning, _ int) string {
			return c.CustomCodeData
		}),
		Descriptions: lo.Map(CustomCodeSignings, func(c *schema.CustomCodeSigning, _ int) string {
			return c.CustomCodeDesc
		}),
		States: lo.Map(CustomCodeSignings, func(c *schema.CustomCodeSigning, _ int) string {
			return string(c.CustomCodeState)
		}),
		Levels: lo.Map(CustomCodeSignings, func(c *schema.CustomCodeSigning, _ int) string {
			return string(c.CustomCodeLevel)
		}),
	}, nil
}
