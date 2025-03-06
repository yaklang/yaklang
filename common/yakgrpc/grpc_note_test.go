package yakgrpc

import (
	"context"
	"fmt"
	"io"
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
	newContent = fmt.Sprintf(`%s
%s
qwer%szxcv`, uuid.NewString(), uuid.NewString(), searchContent)
	index := strings.Index(newContent, searchContent)
	err = createNote(ctx, newTitle, newContent)
	t.Cleanup(func() {
		err := deleteNote(ctx, newTitle)
		require.NoError(t, err)
	})
	require.NoError(t, err)
	searchResp, err := searchNoteContent(ctx, searchContent)
	require.NoError(t, err)
	require.Len(t, searchResp.Data, 1)
	require.Equal(t, fmt.Sprintf("qwer%szxcv", searchContent), strings.TrimSpace(searchResp.Data[0].LineContent))
	require.Equal(t, index, int(searchResp.Data[0].Index))
	require.Equal(t, len(searchContent), int(searchResp.Data[0].Length))
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
