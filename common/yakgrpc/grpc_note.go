package yakgrpc

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"strings"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func (s *Server) CreateNote(ctx context.Context, req *ypb.CreateNoteRequest) (*ypb.CreateNoteResponse, error) {
	id, err := yakit.CreateNote(s.GetProjectDatabase(), req.GetTitle(), req.GetContent())
	if err != nil {
		return nil, err
	}
	return &ypb.CreateNoteResponse{
		Message: &ypb.DbOperateMessage{
			TableName: "note",
			Operation: DbOperationCreate,
		},
		NoteId: int64(id),
	}, nil
}

func (s *Server) UpdateNote(ctx context.Context, req *ypb.UpdateNoteRequest) (*ypb.DbOperateMessage, error) {
	count, err := yakit.UpdateNote(s.GetProjectDatabase(), req.GetFilter(), req.GetUpdateTitle(), req.GetUpdateContent(), req.GetTitle(), req.GetContent())
	return &ypb.DbOperateMessage{
		TableName:  "note",
		Operation:  DbOperationUpdate,
		EffectRows: count,
	}, err
}

func (s *Server) DeleteNote(ctx context.Context, req *ypb.DeleteNoteRequest) (*ypb.DbOperateMessage, error) {
	count, err := yakit.DeleteNote(s.GetProjectDatabase(), req.GetFilter())
	return &ypb.DbOperateMessage{
		TableName:  "note",
		Operation:  DbOperationDelete,
		EffectRows: count,
	}, err
}

func (s *Server) QueryNote(ctx context.Context, req *ypb.QueryNoteRequest) (*ypb.QueryNoteResponse, error) {
	pag, data, err := yakit.QueryNote(s.GetProjectDatabase(), req.GetFilter(), req.GetPagination())
	if err != nil {
		return nil, err
	}

	return &ypb.QueryNoteResponse{
		Pagination: req.GetPagination(),
		Data: lo.Map(data, func(i *schema.Note, _ int) *ypb.Note {
			return i.ToGRPCModel()
		}),
		Total: int64(pag.TotalRecord),
	}, nil
}

func (s *Server) SearchNoteContent(ctx context.Context, req *ypb.SearchNoteContentRequest) (*ypb.SearchNoteContentResponse, error) {
	pag, data, err := yakit.SearchNoteContent(s.GetProjectDatabase(), req.GetKeyword(), req.GetPagination())
	if err != nil {
		return nil, err
	}
	return &ypb.SearchNoteContentResponse{
		Pagination: req.GetPagination(),
		Data:       data,
		Total:      int64(pag.TotalRecord),
	}, nil
}

func (s *Server) ImportNote(req *ypb.ImportNoteRequest, stream ypb.Yak_ImportNoteServer) error {
	zipFile, err := os.Open(req.GetTargetPath())
	if err != nil {
		return fmt.Errorf("failed to open zip file: %v", err)
	}
	defer zipFile.Close()

	stat, err := zipFile.Stat()
	if err != nil {
		return fmt.Errorf("failed to stat zip file: %v", err)
	}
	var (
		count, total float64
	)
	total = float64(stat.Size())

	zipReader, err := zip.NewReader(zipFile, stat.Size())
	if err != nil {
		return fmt.Errorf("failed to create zip reader: %v", err)
	}

	for _, file := range zipReader.File {
		if file.FileInfo().IsDir() {
			continue
		}

		rc, err := file.Open()
		if err != nil {
			return fmt.Errorf("failed to open file in zip: %v", err)
		}
		defer rc.Close()

		content, err := io.ReadAll(rc)
		if err != nil {
			return fmt.Errorf("failed to read file content: %v", err)
		}

		fn := path.Base(file.Name)
		fn = strings.TrimSuffix(fn, path.Ext(fn))

		id, err := yakit.CreateNote(s.GetProjectDatabase(), fn, string(content))
		if err != nil {
			return fmt.Errorf("failed to create note: %v", err)
		}
		count += float64(len(content))

		stream.Send(&ypb.ImportNoteResponse{
			Verbose: fmt.Sprintf("Imported note: %s", fn),
			Percent: count / total,
			NoteId:  int64(id),
		})
	}
	return nil
}

func (s *Server) ExportNote(req *ypb.ExportNoteRequest, stream ypb.Yak_ExportNoteServer) error {
	var (
		count, total float64
	)
	db := yakit.FilterNote(s.GetProjectDatabase(), req.GetFilter())
	ch := bizhelper.YieldModel[*schema.Note](stream.Context(), db, bizhelper.WithYieldModel_CountCallback(func(n int) {
		total = float64(n)
	}))

	zipFile, err := os.Create(req.GetTargetPath())
	if err != nil {
		return fmt.Errorf("failed to create zip file: %v", err)
	}
	defer zipFile.Close()

	zipWriter := zip.NewWriter(zipFile)
	defer zipWriter.Close()

	for note := range ch {
		fileName := fmt.Sprintf("%s.md", note.Title)
		writer, err := zipWriter.Create(fileName)
		if err != nil {
			return fmt.Errorf("failed to create file in zip: %v", err)
		}

		_, err = writer.Write([]byte(note.Content))
		if err != nil {
			return fmt.Errorf("failed to write note content to zip: %v", err)
		}

		count++
		stream.Send(&ypb.ExportNoteResponse{
			Percent: count / total,
		})
	}

	return nil
}
