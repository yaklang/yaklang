package yakgrpc

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"regexp"
	"strings"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// sanitizeFileName 清理文件名中的特殊字符，将不安全的字符替换为下划线
// 主要处理路径分隔符和其他可能导致文件系统问题的字符
func sanitizeFileName(filename string) string {
	if filename == "" {
		return "untitled"
	}

	// 替换 Windows 和 Linux 不允许的字符
	// Windows: <>:"/\|?*
	// Linux: /
	illegalChars := regexp.MustCompile(`[<>:"/\\|?*\x00]`)
	result := illegalChars.ReplaceAllString(filename, "_")

	// 移除开头和结尾的空格、点号
	result = strings.Trim(result, " .")

	// 如果清理后为空，返回默认名称
	if result == "" {
		return "untitled"
	}

	// 将空格替换为下划线
	result = strings.ReplaceAll(result, " ", "_")

	// 移除连续的多个下划线
	re := regexp.MustCompile(`_+`)
	result = re.ReplaceAllString(result, "_")

	// 移除开头和结尾的下划线
	result = strings.Trim(result, "_")

	// 如果清理后为空，返回默认名称
	if result == "" {
		return "untitled"
	}

	return result
}

func (s *Server) CreateNote(ctx context.Context, req *ypb.CreateNoteRequest) (*ypb.CreateNoteResponse, error) {
	// 清理记事本标题中的特殊字符
	safeTitle := sanitizeFileName(req.GetTitle())
	id, err := yakit.CreateNote(s.GetProjectDatabase(), safeTitle, req.GetContent())
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
	// 如果更新标题，清理记事本标题中的特殊字符
	title := req.GetTitle()
	if req.GetUpdateTitle() {
		title = sanitizeFileName(title)
	}
	count, err := yakit.UpdateNote(s.GetProjectDatabase(), req.GetFilter(), req.GetUpdateTitle(), req.GetUpdateContent(), title, req.GetContent())
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

		// 清理文件名中的特殊字符，确保记事本标题安全
		safeTitle := sanitizeFileName(fn)

		id, err := yakit.CreateNote(s.GetProjectDatabase(), safeTitle, string(content))
		if err != nil {
			return fmt.Errorf("failed to create note: %v", err)
		}
		count += float64(len(content))

		stream.Send(&ypb.ImportNoteResponse{
			Verbose: fmt.Sprintf("Imported note: %s", safeTitle),
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

	titleMap := make(map[string]int)

	for note := range ch {
		// 清理文件名中的特殊字符
		safeTitle := sanitizeFileName(note.Title)
		fileName := fmt.Sprintf("%s.md", safeTitle)
		if i, ok := titleMap[fileName]; ok {
			titleMap[fileName] = i + 1
			fileName = fmt.Sprintf("%s(%d).md", safeTitle, i+1)
		} else {
			titleMap[fileName] = 0
		}

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
