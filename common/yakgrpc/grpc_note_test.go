package yakgrpc

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/samber/lo"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func createNote(ctx context.Context, title, content string) error {
	_, err := defaultClient.CreateNote(ctx, &ypb.CreateNoteRequest{
		Title:   title,
		Content: content,
	})
	return err
}

func queryNote(ctx context.Context, titles ...string) (*ypb.QueryNoteResponse, error) {
	return defaultClient.QueryNote(ctx, &ypb.QueryNoteRequest{
		Filter: &ypb.NoteFilter{
			Title: titles,
		},
	})
}

func deleteNote(ctx context.Context, titles ...string) error {
	_, err := defaultClient.DeleteNote(ctx, &ypb.DeleteNoteRequest{
		Filter: &ypb.NoteFilter{
			Title: titles,
		},
	})
	return err
}

func searchNoteContent(ctx context.Context, keyword string) (*ypb.SearchNoteContentResponse, error) {
	return defaultClient.SearchNoteContent(ctx, &ypb.SearchNoteContentRequest{
		Keyword: keyword,
	})
}

func TestNote(t *testing.T) {
	ctx := utils.TimeoutContextSeconds(2)
	// create
	title, content := uuid.NewString(), uuid.NewString()
	err := createNote(ctx, title, content)
	require.NoError(t, err)
	// update
	newTitle, newContent := uuid.NewString(), uuid.NewString()
	_, err = defaultClient.UpdateNote(ctx, &ypb.UpdateNoteRequest{
		Filter: &ypb.NoteFilter{
			Title: []string{title},
		},
		UpdateTitle:   true,
		UpdateContent: true,
		Title:         newTitle,
		Content:       newContent,
	})
	// query
	require.NoError(t, err)
	resp, err := queryNote(ctx, newTitle)
	require.NoError(t, err)
	require.Len(t, resp.Data, 1)
	require.Equal(t, newTitle, resp.Data[0].Title)
	require.Equal(t, newContent, resp.Data[0].Content)
	// delete
	err = deleteNote(ctx, newTitle)
	require.NoError(t, err)
	// query again
	resp, err = queryNote(ctx, newTitle)
	require.NoError(t, err)
	require.Len(t, resp.Data, 0)

	// search
	searchContent := uuid.NewString() + `!@#$%^&*()-_[]\` // special characters
	searchContent1 := fmt.Sprintf("qwer%szxcv", searchContent)
	searchContent2 := fmt.Sprintf("yuio%svbnm", searchContent)
	newContent = fmt.Sprintf(`%s
%s
%[3]s %[3]s
%s`, uuid.NewString(), uuid.NewString(), searchContent1, searchContent2)
	index := strings.Index(newContent, searchContent)
	secondIndex := strings.LastIndex(newContent, searchContent)
	err = createNote(ctx, newTitle, newContent)
	t.Cleanup(func() {
		err := deleteNote(ctx, newTitle)
		require.NoError(t, err)
	})
	require.NoError(t, err)
	searchResp, err := searchNoteContent(ctx, searchContent)
	require.NoError(t, err)
	require.Len(t, searchResp.Data, 2)
	require.Equal(t, fmt.Sprintf("%[1]s %[1]s", searchContent1, searchContent1), strings.TrimSpace(searchResp.Data[0].LineContent))
	require.Equal(t, index, int(searchResp.Data[0].Index))
	require.Equal(t, len(searchContent), int(searchResp.Data[0].Length))
	require.Equal(t, fmt.Sprintf("yuio%svbnm", searchContent), strings.TrimSpace(searchResp.Data[1].LineContent))
	require.Equal(t, secondIndex, int(searchResp.Data[1].Index))
	require.Equal(t, len(searchContent), int(searchResp.Data[1].Length))

	// negative saerch
	searchResp, err = searchNoteContent(ctx, uuid.NewString())
	require.NoError(t, err)
	require.Len(t, searchResp.Data, 0)
}

func TestImportAndExportNote(t *testing.T) {
	ctx := utils.TimeoutContextSeconds(2)
	// create
	titles := make([]string, 0, 3)
	for i := 0; i < cap(titles); i++ {
		title := uuid.NewString()
		titles = append(titles, title)
		err := createNote(ctx, title, uuid.NewString())
		require.NoError(t, err)
	}

	// export
	p := filepath.Join(t.TempDir(), "notes.zip")
	exportStream, err := defaultClient.ExportNote(ctx, &ypb.ExportNoteRequest{
		Filter: &ypb.NoteFilter{
			Title: titles,
		},
		TargetPath: p,
	})
	for {
		_, err := exportStream.Recv()
		if err != nil {
			require.ErrorIs(t, err, io.EOF)
			break
		}
	}
	require.NoError(t, err)

	// delete before import
	err = deleteNote(ctx, titles...)
	require.NoError(t, err)

	// import
	importStream, err := defaultClient.ImportNote(ctx, &ypb.ImportNoteRequest{
		TargetPath: p,
	})
	for {
		_, err := importStream.Recv()
		if err != nil {
			require.ErrorIs(t, err, io.EOF)
			break
		}
	}

	require.NoError(t, err)

	t.Cleanup(func() {
		err := deleteNote(ctx, titles...)
		require.NoError(t, err)
	})
	// query again
	resp, err := queryNote(ctx, titles...)
	require.NoError(t, err)
	require.Len(t, resp.Data, len(titles))
	gotTitles := lo.Map(resp.Data, func(note *ypb.Note, _ int) string {
		return note.Title
	})
	require.ElementsMatch(t, titles, gotTitles)
}

func TestExportNoteWithSameTitle(t *testing.T) {
	ctx := utils.TimeoutContextSeconds(2)
	// create
	title, content := uuid.NewString(), uuid.NewString()
	err := createNote(ctx, title, content)
	require.NoError(t, err)
	err = createNote(ctx, title, content)
	require.NoError(t, err)
	err = createNote(ctx, title, content)
	require.NoError(t, err)

	// export
	p := filepath.Join(t.TempDir(), "notes.zip")
	exportStream, err := defaultClient.ExportNote(ctx, &ypb.ExportNoteRequest{
		Filter: &ypb.NoteFilter{
			Title: []string{title},
		},
		TargetPath: p,
	})
	for {
		_, err := exportStream.Recv()
		if err != nil {
			require.ErrorIs(t, err, io.EOF)
			break
		}
	}
	require.NoError(t, err)

	t.Cleanup(func() {
		err := deleteNote(ctx, title)
		require.NoError(t, err)
	})

	zipFile, err := os.Open(p)
	require.NoError(t, err)
	defer zipFile.Close()

	stat, err := zipFile.Stat()
	require.NoError(t, err)
	zipReader, err := zip.NewReader(zipFile, stat.Size())
	require.NoError(t, err)
	checkFileMap := make(map[string]struct{}, 3)
	for _, file := range zipReader.File {
		if file.FileInfo().IsDir() {
			continue
		}
		rc, err := file.Open()
		require.NoError(t, err)
		defer rc.Close()

		fileContent, err := io.ReadAll(rc)
		require.NoError(t, err)
		require.Equal(t, content, string(fileContent))

		fn := path.Base(file.Name)
		fn = strings.TrimSuffix(fn, path.Ext(fn))
		checkFileMap[fn] = struct{}{}
	}

	require.Len(t, checkFileMap, 3)
	_, ok := checkFileMap[title]
	require.True(t, ok)
	_, ok = checkFileMap[fmt.Sprintf("%s(1)", title)]
	require.True(t, ok)
	_, ok = checkFileMap[fmt.Sprintf("%s(2)", title)]
	require.True(t, ok)
}
