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

func TestGRPCMUSTPASS_Note(t *testing.T) {
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
	require.Equal(t, fmt.Sprintf("yuio%svbnm", searchContent), strings.TrimSpace(searchResp.Data[1].LineContent))
	require.Equal(t, secondIndex, int(searchResp.Data[1].Index))

	// negative saerch
	searchResp, err = searchNoteContent(ctx, uuid.NewString())
	require.NoError(t, err)
	require.Len(t, searchResp.Data, 0)
}

func TestGRPCMUSTPASS_ImportAndExportNote(t *testing.T) {
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

func TestGRPCMUSTPASS_ExportNoteWithSameTitle(t *testing.T) {
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

func TestGRPCMUSTPASS_NoteFileNameSanitization(t *testing.T) {
	ctx := utils.TimeoutContextSeconds(2)

	// 测试包含特殊字符的标题
	specialTitles := []string{
		"test//note",                          // 双斜杠
		"test/note",                           // 单斜杠
		"test\\note",                          // 反斜杠
		"test:note",                           // 冒号
		"test*note",                           // 星号
		"test?note",                           // 问号
		"test\"note",                          // 双引号
		"test<note",                           // 小于号
		"test>note",                           // 大于号
		"test|note",                           // 管道符
		"test//note//with//multiple//slashes", // 多个斜杠
		"   test note   ",                     // 前后空格
		"test___note",                         // 多个下划线
		"",                                    // 空标题
		"   ",                                 // 只有空格
	}

	expectedTitles := []string{
		"test_note",                       // 双斜杠替换为单下划线
		"test_note",                       // 单斜杠替换为下划线
		"test_note",                       // 反斜杠替换为下划线
		"test_note",                       // 冒号替换为下划线
		"test_note",                       // 星号替换为下划线
		"test_note",                       // 问号替换为下划线
		"test_note",                       // 双引号替换为下划线
		"test_note",                       // 小于号替换为下划线
		"test_note",                       // 大于号替换为下划线
		"test_note",                       // 管道符替换为下划线
		"test_note_with_multiple_slashes", // 多个斜杠替换为下划线
		"test_note",                       // 前后空格被移除
		"test_note",                       // 多个下划线合并为单个
		"untitled",                        // 空标题替换为默认值
		"untitled",                        // 只有空格替换为默认值
	}

	// 先清理所有相关的脏数据
	for _, title := range specialTitles {
		_ = deleteNote(ctx, title)
	}
	for _, title := range expectedTitles {
		_ = deleteNote(ctx, title)
	}

	for i, title := range specialTitles {
		content := fmt.Sprintf("content for %s", title)
		err := createNote(ctx, title, content)
		require.NoError(t, err, "Failed to create note with title: %s", title)

		// 查询创建的记事本
		resp, err := queryNote(ctx, expectedTitles[i])
		require.NoError(t, err, "Failed to query note with sanitized title: %s", expectedTitles[i])
		require.Len(t, resp.Data, 1, "Expected exactly one note for title: %s", expectedTitles[i])
		require.Equal(t, expectedTitles[i], resp.Data[0].Title, "Title not properly sanitized for: %s", title)
		require.Equal(t, content, resp.Data[0].Content, "Content not preserved for: %s", title)

		// 清理测试数据
		err = deleteNote(ctx, expectedTitles[i])
		require.NoError(t, err, "Failed to delete note with sanitized title: %s", expectedTitles[i])
	}
}
