package yakgrpc

import (
	"context"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func (s *Server) CreateSnippet(ctx context.Context, req *ypb.SnippetsRequest) (*ypb.Empty, error) {
	if err := yakit.CreateSnippet(s.GetProfileDatabase(), schema.NewSnippets(req)); err != nil {
		return nil, err
	}
	return &ypb.Empty{}, nil
}

func (s *Server) UpdateSnippet(ctx context.Context, req *ypb.EditSnippetsRequest) (*ypb.Empty, error) {
	target := req.GetTarget()

	if err := yakit.UpdateSnippet(s.GetProfileDatabase(), target, &schema.Snippets{
		SnippetId:    "",
		SnippetName:  req.GetName(),
		SnippetBody:  req.GetCode(),
		SnippetDesc:  req.GetDescription(),
		SnippetState: schema.SwitcSnippetsType(req.GetState()),
		SnippetLevel: schema.SnippetsLevel(req.GetLevel()),
	}); err != nil {
		return nil, err
	}
	return &ypb.Empty{}, nil
}

func (s *Server) DeleteSnippets(ctx context.Context, req *ypb.QuerySnippetsRequest) (*ypb.Empty, error) {
	filter := req.GetFilter()

	if err := yakit.DeleteSnippets(s.GetProfileDatabase(), filter); err != nil {
		return nil, err
	}
	return &ypb.Empty{}, nil
}

func (s *Server) QuerySnippets(ctx context.Context, req *ypb.QuerySnippetsRequest) (*ypb.SnippetsResponse, error) {
	filter := req.GetFilter()

	Snippets, err := yakit.QuerySnippets(s.GetProfileDatabase(), filter)
	if err != nil {
		return nil, err
	}
	return &ypb.SnippetsResponse{
		Names: lo.Map(Snippets, func(c *schema.Snippets, _ int) string {
			return c.SnippetName
		}),
		Codes: lo.Map(Snippets, func(c *schema.Snippets, _ int) string {
			return c.SnippetBody
		}),
		Descriptions: lo.Map(Snippets, func(c *schema.Snippets, _ int) string {
			return c.SnippetDesc
		}),
		States: lo.Map(Snippets, func(c *schema.Snippets, _ int) string {
			return string(c.SnippetState)
		}),
		Levels: lo.Map(Snippets, func(c *schema.Snippets, _ int) string {
			return string(c.SnippetLevel)
		}),
	}, nil
}
