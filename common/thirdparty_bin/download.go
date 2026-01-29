package thirdparty_bin

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/utils/progresswriter"
)

// DownloadFile 下载文件
func DownloadFile(url, filename, downloadDir string, options *InstallOptions) (string, error) {
	ctx := context.Background()
	if options != nil && options.Context != nil {
		ctx = options.Context
	}

	// 确保下载目录存在
	if err := os.MkdirAll(downloadDir, 0755); err != nil {
		return "", utils.Errorf("create download directory failed: %v", err)
	}

	filePath := filepath.Join(downloadDir, filename)

	// 检查文件是否已存在且不强制重新下载
	if options == nil || !options.Force {
		if _, err := os.Stat(filePath); err == nil {
			log.Infof("file %s already exists, skipping download", filename)
			return filePath, nil
		}
	}

	// 检查上下文是否已取消
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	default:
	}

	// 发送HEAD请求获取文件大小
	totalSize, err := GetFileSize(url, options)
	if err != nil {
		return "", utils.Errorf("get file size failed: %v", err)
	}

	if options != nil && options.Progress != nil {
		options.Progress(0, 0, totalSize, "开始下载, 文件大小: "+utils.ByteSize(uint64(totalSize)))
	}

	// 检查上下文是否已取消
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	default:
	}

	// 创建临时文件
	tempPath := filePath + ".tmp"
	os.Remove(tempPath) // 删除可能存在的临时文件

	fp, err := os.OpenFile(tempPath, os.O_CREATE|os.O_WRONLY, 0755)
	if err != nil {
		return "", utils.Errorf("create temp file failed: %v", err)
	}

	var downloadError error
	defer func() {
		fp.Close()
		if downloadError != nil {
			os.Remove(tempPath) // 下载失败时删除临时文件
		}
	}()

	// 创建进度追踪器
	prog := progresswriter.New(uint64(totalSize))

	// 启动进度监控goroutine
	progressDone := make(chan struct{})
	if options != nil && options.Progress != nil {
		go func() {
			defer close(progressDone)
			ticker := time.NewTicker(time.Second)
			defer ticker.Stop()

			for {
				select {
				case <-ctx.Done():
					options.Progress(prog.GetPercent(), int64(prog.Count), totalSize, "download cancelled")
					return
				case <-ticker.C:
					options.Progress(prog.GetPercent(), int64(prog.Count), totalSize, "")
					if prog.GetPercent() >= 1 {
						return
					}
				}
			}
		}()
	}

	// 构建GET请求
	isHttps, getRequest, err := lowhttp.ParseUrlToHttpRequestRaw("GET", url)
	if err != nil {
		downloadError = utils.Errorf("parse URL failed: %v", err)
		return "", downloadError
	}

	// 配置lowhttp选项
	opts := []lowhttp.LowhttpOpt{
		lowhttp.WithPacketBytes([]byte(getRequest)),
		lowhttp.WithHttps(isHttps),
		lowhttp.WithContext(ctx),
		lowhttp.WithNoReadMultiResponse(true),
		lowhttp.WithNoFixContentLength(true),
	}

	// 如果提供了代理，添加代理配置
	if options != nil && options.Proxy != "" {
		opts = append(opts, lowhttp.WithProxy(options.Proxy))
	}

	// 添加body stream处理器
	copyDone := make(chan error, 1)
	opts = append(opts, lowhttp.WithBodyStreamReaderHandler(func(r []byte, closer io.ReadCloser) {
		cancelableReader := &cancelableReaderImpl{
			ctx: ctx,
			r:   io.TeeReader(closer, prog),
		}

		go func() {
			_, copyErr := io.Copy(fp, cancelableReader)
			copyDone <- copyErr
		}()

		select {
		case <-ctx.Done():
			downloadError = ctx.Err()
		case err := <-copyDone:
			if err != nil {
				downloadError = utils.Errorf("copy file failed: %v", err)
			}
		}
	}))

	opts = append(opts, lowhttp.WithNoBodyBuffer(true))
	opts = append(opts, lowhttp.WithConnectTimeoutFloat(15.0)) // 设置连接超时
	opts = append(opts, lowhttp.WithTimeout(1800*time.Second)) // 设置读取超时
	// 发送GET请求
	_, err = lowhttp.HTTP(opts...)
	if err != nil && downloadError == nil {
		downloadError = utils.Errorf("HTTP request failed: %v", err)
	}

	// 等待进度监控完成
	if options != nil && options.Progress != nil {
		<-progressDone
	}

	if downloadError != nil {
		return "", downloadError
	}

	// 检查下载是否完整
	fp.Close()
	stat, err := os.Stat(tempPath)
	if err != nil {
		return "", utils.Errorf("check downloaded file failed: %v", err)
	}

	if stat.Size() != totalSize {
		os.Remove(tempPath)
		return "", utils.Errorf("downloaded file size mismatch: expected %d, got %d", totalSize, stat.Size())
	}

	// 移动临时文件到最终位置
	if err := os.Rename(tempPath, filePath); err != nil {
		os.Remove(tempPath)
		return "", utils.Errorf("move temp file failed: %v", err)
	}

	if options != nil && options.Progress != nil {
		options.Progress(1.0, totalSize, totalSize, "download completed")
	}

	log.Infof("file downloaded successfully: %s", filePath)
	return filePath, nil
}

// GetFileSize 获取文件大小
func GetFileSize(url string, options *InstallOptions) (int64, error) {
	ctx := context.Background()
	if options != nil && options.Context != nil {
		ctx = options.Context
	}

	// 构建HEAD请求包
	isHttps, headRequest, err := lowhttp.ParseUrlToHttpRequestRaw("HEAD", url)
	if err != nil {
		return 0, utils.Errorf("parse URL failed: %v", err)
	}

	headRequest = lowhttp.ReplaceHTTPPacketHeader([]byte(headRequest), "Accept", "*/*")

	// 配置lowhttp选项
	opts := []lowhttp.LowhttpOpt{
		lowhttp.WithPacketBytes([]byte(headRequest)),
		lowhttp.WithHttps(isHttps),
		lowhttp.WithContext(ctx),
		lowhttp.WithNoReadMultiResponse(true),
		lowhttp.WithNoFixContentLength(true),
	}

	// 如果提供了代理，添加代理配置
	if options != nil && options.Proxy != "" {
		opts = append(opts, lowhttp.WithProxy(options.Proxy))
	}

	// 发送HEAD请求
	rsp, err := lowhttp.HTTP(opts...)
	if err != nil {
		return 0, utils.Errorf("HEAD request failed: %v", err)
	}

	// 解析Content-Length
	contentLength := lowhttp.GetHTTPPacketHeader(rsp.RawPacket, "Content-Length")
	if contentLength == "" {
		return 0, utils.Errorf("cannot find Content-Length header")
	}

	size, err := strconv.ParseInt(contentLength, 10, 64)
	if err != nil {
		return 0, utils.Errorf("cannot parse Content-Length: %v", err)
	}

	return size, nil
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
