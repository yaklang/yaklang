package yakgrpc

import (
	"context"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func (s *Server) CreateSnippet(ctx context.Context, req *ypb.SnippetsRequest) (*ypb.Empty, error) {
	if err := yakit.CreateSnippet(s.GetProjectDatabase(), schema.NewSnippets(req)); err != nil {
		return nil, err
	}
	return &ypb.Empty{}, nil
}

func (s *Server) UpdateSnippet(ctx context.Context, req *ypb.EditSnippetsRequest) (*ypb.Empty, error) {
	target := req.GetTarget()

	if err := yakit.UpdateSnippet(s.GetProjectDatabase(), target, &schema.Snippets{
		CustomCodeId:    "",
		CustomCodeName:  req.GetName(),
		CustomCodeData:  req.GetCode(),
		CustomCodeDesc:  req.GetDescription(),
		CustomCodeState: schema.SwitcSnippetsType(req.GetState()),
		CustomCodeLevel: schema.SnippetsLevel(req.GetLevel()),
	}); err != nil {
		return nil, err
	}
	return &ypb.Empty{}, nil
}

func (s *Server) DeleteSnippets(ctx context.Context, req *ypb.QuerySnippetsRequest) (*ypb.Empty, error) {
	filter := req.GetFilter()

	if err := yakit.DeleteSnippets(s.GetProjectDatabase(), filter); err != nil {
		return nil, err
	}
	return &ypb.Empty{}, nil
}

func (s *Server) QuerySnippets(ctx context.Context, req *ypb.QuerySnippetsRequest) (*ypb.SnippetsResponse, error) {
	filter := req.GetFilter()

	Snippetss, err := yakit.QuerySnippets(s.GetProjectDatabase(), filter)
	if err != nil {
		return nil, err
	}
	return &ypb.SnippetsResponse{
		Names: lo.Map(Snippetss, func(c *schema.Snippets, _ int) string {
			return c.CustomCodeName
		}),
		Codes: lo.Map(Snippetss, func(c *schema.Snippets, _ int) string {
			return c.CustomCodeData
		}),
		Descriptions: lo.Map(Snippetss, func(c *schema.Snippets, _ int) string {
			return c.CustomCodeDesc
		}),
		States: lo.Map(Snippetss, func(c *schema.Snippets, _ int) string {
			return string(c.CustomCodeState)
		}),
		Levels: lo.Map(Snippetss, func(c *schema.Snippets, _ int) string {
			return string(c.CustomCodeLevel)
		}),
	}, nil
}
