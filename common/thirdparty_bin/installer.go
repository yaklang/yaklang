package thirdparty_bin

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/gobwas/glob"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/utils/progresswriter"
)

// Installer 安装器接口
type Installer interface {
	// Install 安装二进制文件（包含下载）
	Install(descriptor *BinaryDescriptor, options *InstallOptions) error
	// Uninstall 卸载二进制文件
	Uninstall(descriptor *BinaryDescriptor) error
	// GetInstallPath 获取安装路径
	GetInstallPath(descriptor *BinaryDescriptor) string
	// IsInstalled 检查是否已安装
	IsInstalled(descriptor *BinaryDescriptor) bool
	// GetDownloadInfo 获取下载信息
	GetDownloadInfo(descriptor *BinaryDescriptor) (*DownloadInfo, error)
}

// BaseInstaller 基础安装器
type BaseInstaller struct {
	// 默认安装目录
	defaultInstallDir string
	// 下载目录
	downloadDir string
}

// NewInstaller 创建安装器
func NewInstaller(defaultInstallDir, downloadDir string) Installer {
	return &BaseInstaller{
		defaultInstallDir: defaultInstallDir,
		downloadDir:       downloadDir,
	}
}

func (bi *BaseInstaller) Uninstall(descriptor *BinaryDescriptor) error {
	installPath := bi.GetInstallPath(descriptor)
	return os.Remove(installPath)
}

// findMatchingPlatform 查找匹配当前平台的下载信息
func (bi *BaseInstaller) findMatchingPlatform(downloadInfoMap map[string]*DownloadInfo, platformKey string) (*DownloadInfo, string, error) {
	// 首先尝试精确匹配
	if downloadInfo, exists := downloadInfoMap[platformKey]; exists {
		return downloadInfo, platformKey, nil
	}

	// 然后尝试glob模式匹配
	for pattern, downloadInfo := range downloadInfoMap {
		// 编译glob模式
		g, err := glob.Compile(pattern)
		if err != nil {
			// 如果模式编译失败，跳过该模式
			log.Debugf("Failed to compile glob pattern '%s': %v", pattern, err)
			continue
		}

		// 检查模式是否匹配当前平台
		if g.Match(platformKey) {
			log.Debugf("Platform '%s' matched pattern '%s'", platformKey, pattern)
			return downloadInfo, pattern, nil
		}
	}

	return nil, "", utils.Errorf("no download info for platform %s", platformKey)
}
func (bi *BaseInstaller) GetDownloadInfo(descriptor *BinaryDescriptor) (*DownloadInfo, error) {
	sysInfo := GetCurrentSystemInfo()
	platformKey := sysInfo.GetPlatformKey()
	downloadInfo, _, err := bi.findMatchingPlatform(descriptor.DownloadInfoMap, platformKey)
	if err != nil {
		return nil, err
	}
	return downloadInfo, nil
}

// Install 安装二进制文件
func (bi *BaseInstaller) Install(descriptor *BinaryDescriptor, options *InstallOptions) error {
	downloadInfo, err := bi.GetDownloadInfo(descriptor)
	if err != nil {
		return err
	}

	// 判断是否安装
	installed := bi.IsInstalled(descriptor)
	if installed {
		if options.Force {
			err := bi.Uninstall(descriptor)
			if err != nil {
				return utils.Errorf("uninstall failed: %v", err)
			}
		} else {
			log.Infof("binary %s already installed at %s", descriptor.Name, bi.GetInstallPath(descriptor))
			return nil
		}
	}

	installPath := bi.GetInstallPath(descriptor)

	// 确保安装目录存在
	installDir := filepath.Dir(installPath)
	if err := os.MkdirAll(installDir, 0755); err != nil {
		return utils.Errorf("create install directory failed: %v", err)
	}

	// 下载文件
	downloadInfoURL := downloadInfo.URL
	fileChecksum := downloadInfo.Checksums
	pick := downloadInfo.Pick
	filename := GetFilenameFromURL(downloadInfoURL)
	if filename == "" {
		filename = descriptor.Name
	}

	filePath, err := bi.downloadFile(downloadInfoURL, filename, options)
	if err != nil {
		return utils.Errorf("download failed: %v", err)
	}

	// 验证文件校验和
	if fileChecksum != "" {
		if err := bi.verifyFile(filePath, fileChecksum); err != nil {
			return utils.Errorf("file verification failed: %v", err)
		}
		log.Infof("file checksum verified successfully")
	}

	switch descriptor.InstallType {
	case "archive":
		return ExtractFile(filePath, installPath, descriptor.ArchiveType, pick)
	case "bin":
		return os.Rename(filePath, installPath)
	default:
		return utils.Errorf("unknown install type: %s", descriptor.InstallType)
	}

}

// GetInstallPath 获取安装路径
func (bi *BaseInstaller) GetInstallPath(descriptor *BinaryDescriptor) string {
	downloadInfo, err := bi.GetDownloadInfo(descriptor)
	if err != nil {
		return ""
	}
	if downloadInfo.BinPath != "" {
		return filepath.Join(bi.defaultInstallDir, downloadInfo.BinPath)
	}
	return filepath.Join(bi.defaultInstallDir, descriptor.Name)
}

// IsInstalled 检查是否已安装
func (bi *BaseInstaller) IsInstalled(descriptor *BinaryDescriptor) bool {
	_, err := os.Stat(bi.GetInstallPath(descriptor))
	return err == nil
}

// downloadFile 下载文件
func (bi *BaseInstaller) downloadFile(url, filename string, options *InstallOptions) (string, error) {
	ctx := context.Background()
	if options != nil && options.Context != nil {
		ctx = options.Context
	}

	// 确保下载目录存在
	if err := os.MkdirAll(bi.downloadDir, 0755); err != nil {
		return "", utils.Errorf("create download directory failed: %v", err)
	}

	filePath := filepath.Join(bi.downloadDir, filename)

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
	totalSize, err := bi.getFileSize(url, options)
	if err != nil {
		return "", utils.Errorf("get file size failed: %v", err)
	}

	if options != nil && options.Progress != nil {
		options.Progress(0, 0, totalSize, "开始下载")
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
					options.Progress(prog.GetPercent(), int64(prog.Count), totalSize, "downloading...")
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
		lowhttp.WithTryReadMultiResponse(false),
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

// getFileSize 获取文件大小
func (bi *BaseInstaller) getFileSize(url string, options *InstallOptions) (int64, error) {
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
		lowhttp.WithTryReadMultiResponse(false),
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

// verifyFile 验证文件校验和
func (bi *BaseInstaller) verifyFile(filePath, expectedChecksum string) error {
	if expectedChecksum == "" {
		return nil // 没有提供校验和，跳过验证
	}

	file, err := os.Open(filePath)
	if err != nil {
		return utils.Errorf("open file failed: %v", err)
	}
	defer file.Close()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return utils.Errorf("calculate checksum failed: %v", err)
	}

	actualChecksum := fmt.Sprintf("%x", hasher.Sum(nil))
	if actualChecksum != expectedChecksum {
		return utils.Errorf("checksum mismatch: expected %s, got %s", expectedChecksum, actualChecksum)
	}

	return nil
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
