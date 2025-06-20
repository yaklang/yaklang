package yakgrpc

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
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
	client := utils.NewDefaultHTTPClientWithProxy(proxy)
	client.Timeout = time.Hour

	// 使用带上下文的HEAD请求
	req, err := http.NewRequestWithContext(ctx, "HEAD", targetUrl, nil)
	if err != nil {
		return err
	}
	rsp, err := client.Do(req)
	if err != nil {
		return err
	}
	rsp.Body.Close()

	i, err := strconv.Atoi(rsp.Header.Get("Content-Length"))
	if err != nil {
		return utils.Errorf("cannot fetch cl: %v", err)
	}
	info(0, "共需下载大小为：Download %v Total", utils.ByteSize(uint64(i)))

	// 检查上下文是否已取消
	select {
	case <-ctx.Done():
		info(0, "下载已取消: Download Cancelled")
		return ctx.Err()
	default:
	}

	// 使用带上下文的GET请求
	req, err = http.NewRequestWithContext(ctx, "GET", targetUrl, nil)
	if err != nil {
		return err
	}
	rsp, err = client.Do(req)
	if err != nil {
		return utils.Errorf("download material failed: %s", err)
	}
	defer rsp.Body.Close()

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

	// 创建可取消的reader
	cancelableReader := &cancelableReaderImpl{
		ctx: ctx,
		r:   io.TeeReader(rsp.Body, prog),
	}

	// 在goroutine中执行复制操作
	copyDone := make(chan error, 1)
	go func() {
		_, copyErr := io.Copy(fp, cancelableReader)
		copyDone <- copyErr
	}()

	// 等待下载完成或上下文取消
	select {
	case <-ctx.Done():
		// 上下文取消，清理文件
		fp.Close()
		os.Remove(fPath)
		info(0, "下载已取消，文件已清理: Download Cancelled, File Cleaned")
		return ctx.Err()
	case err := <-copyDone:
		if err != nil {
			info(0, "下载文件失败: Download Failed: %s", err)
			return err
		}
		info(100, "下载文件成功：Download Finished")
		return nil
	}
}

// cancelableReaderImpl 实现可取消的Reader
type cancelableReaderImpl struct {
	ctx context.Context
	r   io.Reader
}

func (cr *cancelableReaderImpl) Read(p []byte) (n int, err error) {
	// 检查上下文是否取消
	select {
	case <-cr.ctx.Done():
		return 0, cr.ctx.Err()
	default:
	}

	// 使用goroutine执行实际读取，以便能响应上下文取消
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
