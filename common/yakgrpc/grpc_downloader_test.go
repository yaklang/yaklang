package yakgrpc

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// MockDownloadStream 模拟下载流
type MockDownloadStream struct {
	grpc.ServerStream
	ctx      context.Context
	messages []*ypb.ExecResult
}

func NewMockDownloadStream(ctx context.Context) *MockDownloadStream {
	return &MockDownloadStream{
		ctx:      ctx,
		messages: make([]*ypb.ExecResult, 0),
	}
}

func (m *MockDownloadStream) Context() context.Context {
	return m.ctx
}

func (m *MockDownloadStream) Send(result *ypb.ExecResult) error {
	m.messages = append(m.messages, result)
	return nil
}

func (m *MockDownloadStream) GetMessages() []*ypb.ExecResult {
	return m.messages
}

// 实现grpc.ServerStream接口的方法
func (m *MockDownloadStream) SetHeader(metadata.MD) error {
	return nil
}

func (m *MockDownloadStream) SendHeader(metadata.MD) error {
	return nil
}

func (m *MockDownloadStream) SetTrailer(metadata.MD) {
}

func (m *MockDownloadStream) SendMsg(msg interface{}) error {
	return nil
}

func (m *MockDownloadStream) RecvMsg(msg interface{}) error {
	return nil
}

func TestDownloadWithStream(t *testing.T) {
	// 创建测试数据
	testData := strings.Repeat("Hello, World! ", 1000) // 约13KB的测试数据

	// 创建mock HTTP服务器
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "HEAD":
			w.Header().Set("Content-Length", fmt.Sprintf("%d", len(testData)))
			w.WriteHeader(http.StatusOK)
		case "GET":
			w.Header().Set("Content-Length", fmt.Sprintf("%d", len(testData)))
			w.Header().Set("Content-Type", "text/plain")
			w.WriteHeader(http.StatusOK)

			// 模拟慢速下载，每次写入一小部分数据
			data := []byte(testData)
			chunkSize := 100
			for i := 0; i < len(data); i += chunkSize {
				end := i + chunkSize
				if end > len(data) {
					end = len(data)
				}
				w.Write(data[i:end])
				if f, ok := w.(http.Flusher); ok {
					f.Flush()
				}
				time.Sleep(10 * time.Millisecond) // 模拟网络延迟
			}
		}
	}))
	defer server.Close()

	t.Run("SuccessfulDownload", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		stream := NewMockDownloadStream(ctx)
		s := &Server{}

		filename := "test-success.txt"
		fileGetter := func() (string, string, error) {
			return server.URL, filename, nil
		}

		err := s.DownloadWithStream("", fileGetter, stream)
		require.NoError(t, err)

		// 验证文件是否下载成功
		expectedPath := filepath.Join(consts.GetDefaultYakitProjectsDir(), "libs", filename)
		_, err = os.Stat(expectedPath)
		assert.NoError(t, err, "下载的文件应该存在")

		// 验证文件内容
		content, err := os.ReadFile(expectedPath)
		require.NoError(t, err)
		assert.Equal(t, testData, string(content), "文件内容应该匹配")

		// 验证消息
		messages := stream.GetMessages()
		assert.Greater(t, len(messages), 0, "应该有进度消息")

		// 查找完成消息
		found := false
		for _, msg := range messages {
			if strings.Contains(string(msg.Message), "下载文件成功") {
				found = true
				break
			}
		}
		assert.True(t, found, "应该有下载成功的消息")

		// 清理文件
		os.Remove(expectedPath)
	})

	t.Run("CancelledDownload", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())

		stream := NewMockDownloadStream(ctx)
		s := &Server{}

		filename := "test-cancelled.txt"
		fileGetter := func() (string, string, error) {
			return server.URL, filename, nil
		}

		// 启动下载
		downloadDone := make(chan error, 1)
		go func() {
			err := s.DownloadWithStream("", fileGetter, stream)
			downloadDone <- err
		}()

		// 等待一小段时间让下载开始
		time.Sleep(100 * time.Millisecond)

		// 取消下载
		cancel()

		// 等待下载完成或超时
		select {
		case err := <-downloadDone:
			assert.Error(t, err, "取消的下载应该返回错误")
			assert.Equal(t, context.Canceled, err, "错误应该是context.Canceled")
		case <-time.After(5 * time.Second):
			t.Fatal("下载没有在预期时间内响应取消")
		}

		// 验证文件是否被清理
		expectedPath := filepath.Join(consts.GetDefaultYakitProjectsDir(), "libs", filename)
		_, err := os.Stat(expectedPath)
		assert.True(t, os.IsNotExist(err), "取消下载后文件应该被删除")

		// 验证取消消息
		messages := stream.GetMessages()
		found := false
		for _, msg := range messages {
			if strings.Contains(string(msg.Message), "下载已取消") ||
				strings.Contains(string(msg.Message), "Download Cancelled") {
				found = true
				break
			}
		}
		assert.True(t, found, "应该有下载取消的消息")
	})

	t.Run("SlowServerCancellation", func(t *testing.T) {
		// 创建一个非常慢的服务器来测试取消功能
		slowServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case "HEAD":
				w.Header().Set("Content-Length", "10000")
				w.WriteHeader(http.StatusOK)
			case "GET":
				w.Header().Set("Content-Length", "10000")
				w.WriteHeader(http.StatusOK)

				// 模拟非常慢的响应
				for i := 0; i < 100; i++ {
					select {
					case <-r.Context().Done():
						return // 服务器端检测到客户端取消
					default:
						w.Write([]byte("x"))
						if f, ok := w.(http.Flusher); ok {
							f.Flush()
						}
						time.Sleep(100 * time.Millisecond) // 每100ms写入1字节
					}
				}
			}
		}))
		defer slowServer.Close()

		ctx, cancel := context.WithCancel(context.Background())

		stream := NewMockDownloadStream(ctx)
		s := &Server{}

		filename := "test-slow-cancel.txt"
		fileGetter := func() (string, string, error) {
			return slowServer.URL, filename, nil
		}

		// 启动下载
		downloadDone := make(chan error, 1)
		go func() {
			err := s.DownloadWithStream("", fileGetter, stream)
			downloadDone <- err
		}()

		// 等待下载开始
		time.Sleep(200 * time.Millisecond)

		// 取消下载
		cancel()

		// 验证下载快速响应取消
		select {
		case err := <-downloadDone:
			assert.Error(t, err, "取消的下载应该返回错误")
			assert.Equal(t, context.Canceled, err, "错误应该是context.Canceled")
		case <-time.After(2 * time.Second):
			t.Fatal("下载没有在2秒内响应取消")
		}

		// 验证文件被清理
		expectedPath := filepath.Join(consts.GetDefaultYakitProjectsDir(), "libs", filename)
		_, err := os.Stat(expectedPath)
		assert.True(t, os.IsNotExist(err), "取消下载后文件应该被删除")
	})
}
