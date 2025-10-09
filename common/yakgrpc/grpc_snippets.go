package yakgrpc

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func (s *Server) CreateSnippet(ctx context.Context, req *ypb.SnippetsRequest) (*ypb.Empty, error) {
	if err := yakit.CreateSnippet(s.GetProfileDatabase(), schema.NewSnippets(req)); err != nil {
		return &ypb.Empty{}, err
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
		return &ypb.Empty{}, err
	}
	return &ypb.Empty{}, nil
}

func (s *Server) DeleteSnippets(ctx context.Context, req *ypb.QuerySnippetsRequest) (*ypb.Empty, error) {
	filter := req.GetFilter()

	if err := yakit.DeleteSnippets(s.GetProfileDatabase(), filter); err != nil {
		return &ypb.Empty{}, err
	}
	return &ypb.Empty{}, nil
}

func (s *Server) QuerySnippets(ctx context.Context, req *ypb.QuerySnippetsRequest) (*ypb.SnippetsResponse, error) {
	filter := req.GetFilter()

	Snippets, err := yakit.QuerySnippets(s.GetProfileDatabase(), filter)
	if err != nil {
		return &ypb.SnippetsResponse{}, err
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

// VSCodeSnippet 表示 VSCode 格式的代码片段
type VSCodeSnippet struct {
	Scope       string   `json:"scope"`
	Prefix      string   `json:"prefix"`
	Body        []string `json:"body"`
	Description string   `json:"description"`
}

func (s *Server) ShowSnippetsWithJson(ctx context.Context, req *ypb.Empty) (*ypb.SnippetsJsonResponse, error) {
	snippets, err := yakit.GetAllSnippetss(s.GetProfileDatabase())
	if err != nil {
		return nil, err
	}

	vscodeSnippets := make(map[string]VSCodeSnippet)
	for _, snippet := range snippets {
		bodyLines := strings.Split(snippet.SnippetBody, "\n")

		vscodeSnippets[snippet.SnippetName] = VSCodeSnippet{
			Scope:       string(snippet.SnippetState), // http/yak
			Prefix:      snippet.SnippetName,
			Body:        bodyLines,
			Description: snippet.SnippetDesc,
		}
	}

	jsonData, err := json.MarshalIndent(vscodeSnippets, "", "    ")
	if err != nil {
		return nil, err
	}

	return &ypb.SnippetsJsonResponse{
		JsonData: string(jsonData),
	}, nil
}

func (s *Server) ImportSnippetsFromJson(ctx context.Context, req *ypb.ImportSnippetsRequest) (*ypb.Empty, error) {
	if req.GetJsonData() == "" {
		return &ypb.Empty{}, utils.Errorf("JSON data cannot be empty")
	}

	var vscodeSnippets map[string]VSCodeSnippet
	if err := json.Unmarshal([]byte(req.GetJsonData()), &vscodeSnippets); err != nil {
		return &ypb.Empty{}, utils.Errorf("failed to parse JSON: %v", err)
	}

	db := s.GetProfileDatabase()

	if err := yakit.DeleteAllSnippets(db); err != nil {
		return &ypb.Empty{}, utils.Errorf("failed to clear existing snippets: %v", err)
	}

	for name, vscodeSnippet := range vscodeSnippets {
		snippetName := name
		if vscodeSnippet.Prefix != "" {
			snippetName = vscodeSnippet.Prefix
		}

		snippetBody := strings.Join(vscodeSnippet.Body, "\n")

		snippet := &schema.Snippets{
			SnippetName:  snippetName,
			SnippetBody:  snippetBody,
			SnippetDesc:  vscodeSnippet.Description,
			SnippetState: schema.SwitcSnippetsType(vscodeSnippet.Scope),
			SnippetLevel: schema.Snippets_Level_Snippet, // Default level
		}

		if err := yakit.CreateSnippet(db, snippet); err != nil {
			return &ypb.Empty{}, utils.Errorf("failed to create snippet '%s': %v", snippetName, err)
		}
	}

	return &ypb.Empty{}, nil
}
