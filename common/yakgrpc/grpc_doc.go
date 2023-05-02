package yakgrpc

import (
	"context"
	"yaklang.io/yaklang/common/utils"
	"yaklang.io/yaklang/common/yakgrpc/yakit"
	"yaklang.io/yaklang/common/yakgrpc/ypb"
)

func (s *Server) SaveMarkdownDocument(ctx context.Context, req *ypb.SaveMarkdownDocumentRequest) (*ypb.Empty, error) {
	err := yakit.CreateOrUpdateMarkdownDoc(s.GetProfileDatabase(), req.GetYakScriptId(), req.GetYakScriptName(), &ypb.SaveMarkdownDocumentRequest{
		YakScriptName: req.GetYakScriptName(),
		YakScriptId:   req.GetYakScriptId(),
		Markdown:      req.GetMarkdown(),
	})
	if err != nil {
		return nil, err
	}
	return &ypb.Empty{}, nil
}

func (s *Server) GetMarkdownDocument(ctx context.Context, req *ypb.GetMarkdownDocumentRequest) (*ypb.GetMarkdownDocumentResponse, error) {
	markdown, err := yakit.GetMarkdownDocByName(s.GetProfileDatabase(), req.GetYakScriptId(), req.GetYakScriptName())
	if err != nil {
		return nil, err
	}

	ins, _ := yakit.GetYakScript(s.GetProfileDatabase(), req.GetYakScriptId())
	if ins != nil {
		return &ypb.GetMarkdownDocumentResponse{
			Script:   ins.ToGRPCModel(),
			Markdown: utils.EscapeInvalidUTF8Byte([]byte(markdown.Markdown)),
		}, nil
	}

	ins, _ = yakit.GetYakScriptByName(s.GetProfileDatabase(), req.GetYakScriptName())
	if ins != nil {
		return &ypb.GetMarkdownDocumentResponse{
			Script:   ins.ToGRPCModel(),
			Markdown: utils.EscapeInvalidUTF8Byte([]byte(markdown.Markdown)),
		}, nil
	}
	return nil, err
}

func (s *Server) DeleteMarkdownDocument(ctx context.Context, req *ypb.GetMarkdownDocumentRequest) (*ypb.Empty, error) {
	markdown, err := yakit.GetMarkdownDocByName(s.GetProfileDatabase(), req.GetYakScriptId(), req.GetYakScriptName())
	if err != nil {
		return nil, err
	}

	err = yakit.DeleteMarkdownDocByID(s.GetProfileDatabase(), int64(markdown.ID))
	if err != nil {
		return nil, err
	}
	return &ypb.Empty{}, nil
}
