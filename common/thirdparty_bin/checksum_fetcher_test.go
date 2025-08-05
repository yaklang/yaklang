package thirdparty_bin

import (
	"encoding/base64"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/utils/lowhttp"
)

// TestFetchAllMD5 测试获取所有URL的MD5值
func TestFetchAllMD5(t *testing.T) {
	// 加载配置
	config, err := LoadConfigFromEmbedded()
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// 新的baseurl
	newBaseURL := "https://yaklang.oss-cn-beijing.aliyuncs.com"

	fmt.Printf("=== Fetching MD5 for all URLs ===\n")
	fmt.Printf("Original baseurl: %s\n", config.BaseURL)
	fmt.Printf("New baseurl: %s\n\n", newBaseURL)

	// 收集所有需要处理的URL
	type urlInfo struct {
		binaryName   string
		platform     string
		originalURL  string
		relativePath string
		fullURL      string
	}

	var urlList []urlInfo

	// 遍历所有二进制工具
	for _, binary := range config.Binaries {
		for platform, downloadInfo := range binary.DownloadInfoMap {
			originalURL := downloadInfo.URL

			// 计算相对路径（去掉原始baseurl）
			var relativePath string
			if strings.HasPrefix(originalURL, config.BaseURL) {
				relativePath = strings.TrimPrefix(originalURL, config.BaseURL)
				relativePath = strings.TrimPrefix(relativePath, "/")
			} else if strings.HasPrefix(originalURL, "/") {
				relativePath = strings.TrimPrefix(originalURL, "/")
			} else if !strings.HasPrefix(originalURL, "http://") && !strings.HasPrefix(originalURL, "https://") {
				relativePath = originalURL
			} else {
				// 跳过外部URL
				continue
			}

			// 构建新的完整URL
			fullURL := newBaseURL + "/" + relativePath

			urlList = append(urlList, urlInfo{
				binaryName:   binary.Name,
				platform:     platform,
				originalURL:  originalURL,
				relativePath: relativePath,
				fullURL:      fullURL,
			})
		}
	}

	fmt.Printf("Found %d URLs to process\n\n", len(urlList))

	// 处理每个URL
	for i, info := range urlList {
		fmt.Printf("[%d/%d] %s (%s)\n", i+1, len(urlList), info.binaryName, info.platform)
		fmt.Printf("  Path: %s\n", info.relativePath)
		fmt.Printf("  URL:  %s\n", info.fullURL)

		// 获取MD5
		md5Hash, err := fetchContentMD5(info.fullURL)
		if err != nil {
			fmt.Printf("  Error: %v\n", err)
		} else if md5Hash != "" {
			fmt.Printf("  MD5:  %s\n", md5Hash)
		} else {
			fmt.Printf("  MD5:  (no Content-MD5 header)\n")
		}
		fmt.Println()
	}

	// 输出YAML格式的MD5配置
	fmt.Printf("\n=== YAML Configuration with MD5 ===\n\n")

	currentBinary := ""
	for _, info := range urlList {
		if info.binaryName != currentBinary {
			if currentBinary != "" {
				fmt.Println()
			}
			fmt.Printf("# %s\n", info.binaryName)
			currentBinary = info.binaryName
		}

		md5Hash, err := fetchContentMD5(info.fullURL)
		if err != nil {
			fmt.Printf("#   %s: # Error: %v\n", info.platform, err)
		} else if md5Hash != "" {
			fmt.Printf("#   %s:\n#     md5: \"%s\"\n", info.platform, md5Hash)
		} else {
			fmt.Printf("#   %s: # No Content-MD5 header\n", info.platform)
		}
	}
}

// fetchContentMD5 发送HEAD请求并提取Content-MD5头部，转换为hex格式
func fetchContentMD5(url string) (string, error) {
	// 构建HEAD请求
	isHttps, headRequest, err := lowhttp.ParseUrlToHttpRequestRaw("HEAD", url)
	if err != nil {
		return "", fmt.Errorf("parse URL failed: %v", err)
	}

	// 设置请求头
	headRequest = lowhttp.ReplaceHTTPPacketHeader([]byte(headRequest), "Accept", "*/*")
	headRequest = lowhttp.ReplaceHTTPPacketHeader([]byte(headRequest), "User-Agent", "Yaklang-MD5-Fetcher/1.0")

	// 配置lowhttp选项
	opts := []lowhttp.LowhttpOpt{
		lowhttp.WithPacketBytes([]byte(headRequest)),
		lowhttp.WithHttps(isHttps),
		lowhttp.WithTimeout(30 * time.Second),
		lowhttp.WithNoReadMultiResponse(true),
		lowhttp.WithNoFixContentLength(true),
	}

	// 发送HEAD请求
	rsp, err := lowhttp.HTTP(opts...)
	if err != nil {
		return "", fmt.Errorf("HEAD request failed: %v", err)
	}

	// 检查状态码（从原始响应包中解析）
	statusLine := string(rsp.RawPacket)
	if !strings.Contains(statusLine, "200 OK") && !strings.Contains(statusLine, "HTTP/1.1 200") && !strings.Contains(statusLine, "HTTP/1.0 200") {
		// 尝试提取状态码
		lines := strings.Split(statusLine, "\n")
		if len(lines) > 0 {
			return "", fmt.Errorf("HTTP error: %s", strings.TrimSpace(lines[0]))
		}
		return "", fmt.Errorf("HTTP request failed")
	}

	// 提取Content-MD5头部
	contentMD5 := lowhttp.GetHTTPPacketHeader(rsp.RawPacket, "Content-MD5")
	if contentMD5 == "" {
		return "", nil // 没有Content-MD5头部
	}

	// Content-MD5是base64编码的MD5哈希，需要转换为hex格式
	md5Bytes, err := base64.StdEncoding.DecodeString(contentMD5)
	if err != nil {
		return "", fmt.Errorf("decode base64 Content-MD5 failed: %v", err)
	}

	// 转换为hex格式
	md5Hex := fmt.Sprintf("%x", md5Bytes)

	return md5Hex, nil
}

// TestSingleURL 测试单个URL（用于调试）
func TestSingleURL(t *testing.T) {
	testURL := "https://yaklang.oss-cn-beijing.aliyuncs.com/vulinbox/latest/vulinbox_linux_amd64"

	fmt.Printf("Testing single URL: %s\n", testURL)

	md5Hash, err := fetchContentMD5(testURL)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	if md5Hash != "" {
		fmt.Printf("MD5: %s\n", md5Hash)
	} else {
		fmt.Println("No Content-MD5 header found")
	}
}
