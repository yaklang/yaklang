package yakgrpc

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/utils/progresswriter"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func (s *Server) DownloadWithStream(proxy string, fileGetter func() (urlStr string, name string, err error), stream DownloadStream) error {
	if fileGetter == nil {
		return utils.Error("fileGetter is nil")
	}
	info := func(progress float64, s string, items ...interface{}) {
		var msg string
		if len(items) > 0 {
			msg = fmt.Sprintf(s, items)
		} else {
			msg = s
		}
		log.Info(msg)
		progressInfo, _ := strconv.ParseFloat(fmt.Sprintf("%.2f", progress), 64)
		stream.Send(&ypb.ExecResult{
			IsMessage: true,
			Message:   []byte(msg),
			Progress:  float32(progressInfo),
		})
	}

	var targetUrl, filename, err = fileGetter()
	if err != nil {
		return utils.Errorf("cannot get file: %v", err)
	}

	ctx := stream.Context()

	// 检查上下文是否已取消
	select {
	case <-ctx.Done():
		info(0, "下载已取消: Download Cancelled")
		return ctx.Err()
	default:
	}

	info(0, "获取下载材料大小: Fetching Download Material Basic Info")

	// 构建HEAD请求包
	isHttps, headRequest, err := lowhttp.ParseUrlToHttpRequestRaw("HEAD", targetUrl)
	if err != nil {
		return utils.Errorf("parse URL failed: %v", err)
	}
	// 配置lowhttp选项
	opts := []lowhttp.LowhttpOpt{
		lowhttp.WithPacketBytes([]byte(headRequest)),
		lowhttp.WithHttps(isHttps),
		lowhttp.WithContext(ctx),
		lowhttp.WithSaveHTTPFlow(false), // 禁用 HTTP 流保存
	}

	// 如果提供了代理，添加代理配置
	if proxy != "" {
		opts = append(opts, lowhttp.WithProxy(proxy))
	}

	// 发送HEAD请求获取文件大小
	rsp, err := lowhttp.HTTPWithoutRedirect(opts...)
	if err != nil {
		return utils.Errorf("HEAD request failed: %v", err)
	}

	// 解析Content-Length
	contentLength := lowhttp.GetHTTPPacketHeader(rsp.RawPacket, "Content-Length")

	if contentLength == "" {
		return utils.Errorf("cannot find Content-Length header")
	}

	i, err := strconv.Atoi(contentLength)
	if err != nil {
		return utils.Errorf("cannot parse Content-Length: %v", err)
	}
	info(0, "共需下载大小为：Download %v Total", utils.ByteSize(uint64(i)))

	// 检查上下文是否已取消
	select {
	case <-ctx.Done():
		info(0, "下载已取消: Download Cancelled")
		return ctx.Err()
	default:
	}

	dirPath := filepath.Join(
		consts.GetDefaultYakitProjectsDir(),
		"libs",
	)
	err = os.MkdirAll(dirPath, 0755)
	if err != nil {
		return err
	}
	fPath := filepath.Join(dirPath, filename)
	os.RemoveAll(fPath)
	fp, err := os.OpenFile(fPath, os.O_CREATE|os.O_WRONLY, 0755)
	if err != nil {
		return err
	}
	defer fp.Close()

	prog := progresswriter.New(uint64(i))

	// 启动进度监控goroutine
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				info(prog.GetPercent()*100, "下载已取消: Download Cancelled")
				return
			case <-ticker.C:
				info(prog.GetPercent()*100, "")
				if prog.GetPercent() >= 1 {
					return
				}
			}
		}
	}()

	isEndCtx, sendEnd := context.WithCancel(context.Background())
	defer sendEnd()
	// 构建GET请求包
	isHttps, getRequest, err := lowhttp.ParseUrlToHttpRequestRaw("GET", targetUrl)
	if err != nil {
		return utils.Errorf("parse URL failed: %v", err)
	}

	var downloadError error
	// 使用相同的选项配置GET请求
	opts = []lowhttp.LowhttpOpt{
		lowhttp.WithPacketBytes([]byte(getRequest)),
		lowhttp.WithHttps(isHttps),
		lowhttp.WithContext(ctx),
		lowhttp.WithSaveHTTPFlow(false), // 禁用 HTTP 流保存到数据库，避免大文件占用内存
		lowhttp.WithNoBodyBuffer(true),  // 禁用响应体缓冲，避免大文件占用内存
	}
	if proxy != "" {
		opts = append(opts, lowhttp.WithProxy(proxy))
	}
	opts = append(opts, lowhttp.WithBodyStreamReaderHandler(func(r []byte, closer io.ReadCloser) {
		cancelableReader := &cancelableReaderImpl{
			ctx: ctx,
			r:   io.TeeReader(closer, prog),
		}

		copyDone := make(chan error, 1)
		go func() {
			_, copyErr := io.Copy(fp, cancelableReader)
			copyDone <- copyErr
		}()

		select {
		case <-ctx.Done():
			fp.Close()
			os.Remove(fPath)
			info(0, "下载已取消，文件已清理: Download Cancelled, File Cleaned")
			downloadError = ctx.Err()
			sendEnd()
		case err := <-copyDone:
			if err != nil {
				info(0, "下载文件失败: Download Failed: %s", err)
				downloadError = err
			}
			info(100, "下载文件成功：Download Finished")
			sendEnd()
		}
	}))

	_, err = lowhttp.HTTPWithoutRedirect(opts...)
	if err != nil {
		return err
	}
	<-isEndCtx.Done()
	return downloadError
}

// cancelableReaderImpl 实现可取消的Reader
type cancelableReaderImpl struct {
	ctx context.Context
	r   io.Reader
}

func (cr *cancelableReaderImpl) Read(p []byte) (n int, err error) {
	select {
	case <-cr.ctx.Done():
		return 0, cr.ctx.Err()
	default:
	}

	done := make(chan struct{})
	var readN int
	var readErr error

	go func() {
		readN, readErr = cr.r.Read(p)
		close(done)
	}()

	select {
	case <-cr.ctx.Done():
		return 0, cr.ctx.Err()
	case <-done:
		return readN, readErr
	}
}
