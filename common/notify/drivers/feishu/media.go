package feishu

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/notify"
	"github.com/yaklang/yaklang/common/notify/internal/httpclient"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
)

// maxAttachmentBytes 单个附件最大字节数（25MB，与参考项目一致）。
const maxAttachmentBytes = 25 * 1024 * 1024

// allowedImageMIME 图片 MIME 白名单。
var allowedImageMIME = map[string]bool{
	"image/jpeg": true,
	"image/png":  true,
	"image/webp": true,
	"image/gif":  true,
}

// DownloadResource 下载飞书消息资源（图片/文件）到本地临时目录。
//
// GET /open-apis/im/v1/messages/{message_id}/resources/{file_key}?type={image|file}
// Bearer token，binary 响应。落盘到 consts.TempAIDir("im-media") 下。
// 超过 maxAttachmentBytes 的文件会被删除并返回 error。
//
// 参数：
//   - cfg: 发送配置（含 AppID/AppSecret/BaseURL，用于取 token + 拼 URL）
//   - messageID: 飞书消息 ID（om_xxx）
//   - fileKey: image_key / file_key
//   - isImage: true=type=image, false=type=file
//
// 返回：localPath, mimeType, size, err
func (c *Client) DownloadResource(cfg *notify.SendConfig, messageID, fileKey string, isImage bool) (localPath, mimeType string, size int64, err error) {
	if messageID == "" {
		return "", "", 0, fmt.Errorf("feishu: message_id is required for download")
	}
	if fileKey == "" {
		return "", "", 0, fmt.Errorf("feishu: file_key is required for download")
	}
	token, err := c.tokens.getToken()
	if err != nil {
		return "", "", 0, fmt.Errorf("feishu: get token for download: %w", err)
	}

	resourceType := "file"
	if isImage {
		resourceType = "image"
	}
	url := c.base() + "/open-apis/im/v1/messages/" + url.PathEscape(messageID) + "/resources/" + url.PathEscape(fileKey)
	query := map[string]string{"type": resourceType}
	headers := map[string]string{"Authorization": "Bearer " + token}

	raw, err := httpclient.Request("GET", url, headers, query, nil, buildHTTPOpts(cfg.Proxy, cfg.Timeout)...)
	if err != nil {
		return "", "", 0, fmt.Errorf("feishu: download resource %s: %w", fileKey, err)
	}

	statusCode := lowhttp.GetStatusCodeFromResponse(raw)
	if statusCode != 200 {
		body := lowhttp.GetHTTPPacketBody(raw)
		return "", "", 0, fmt.Errorf("feishu: download status %d: %s", statusCode, string(body))
	}

	// 检查是否是错误响应（JSON，非 binary）
	body := lowhttp.GetHTTPPacketBody(raw)
	mimeType = lowhttp.GetHTTPPacketHeader(raw, "Content-Type")
	if idx := strings.Index(mimeType, ";"); idx >= 0 {
		mimeType = strings.TrimSpace(mimeType[:idx])
	}
	// 飞书错误时返回 JSON，Content-Type: application/json
	if strings.HasPrefix(mimeType, "application/json") {
		return "", "", 0, fmt.Errorf("feishu: download returned JSON error: %s", string(body))
	}

	// 图片 MIME 白名单
	if isImage && !allowedImageMIME[mimeType] {
		return "", "", 0, fmt.Errorf("feishu: image mime %q not in whitelist", mimeType)
	}

	// 大小检查
	size = int64(len(body))
	if size > maxAttachmentBytes {
		return "", "", 0, fmt.Errorf("feishu: attachment too large: %d bytes (max %d)", size, maxAttachmentBytes)
	}

	// 落盘
	mediaDir := consts.TempAIDir("im-media")
	ext := mimeToExt(mimeType, isImage, fileKey)
	randBytes := make([]byte, 8)
	_, _ = rand.Read(randBytes)
	fileName := fmt.Sprintf("%s.%s", hex.EncodeToString(randBytes), ext)
	localPath = filepath.Join(mediaDir, fileName)
	if err := os.WriteFile(localPath, body, 0644); err != nil {
		return "", "", 0, fmt.Errorf("feishu: write media file: %w", err)
	}

	log.Debugf("feishu: downloaded %s resource to %s (%d bytes, mime=%s)", resourceType, localPath, size, mimeType)
	return localPath, mimeType, size, nil
}

// mimeToExt 根据 MIME 类型推断文件扩展名；无法识别时从 fileKey/fileName 推断或默认 bin。
func mimeToExt(mimeType string, isImage bool, fileKey string) string {
	switch mimeType {
	case "image/jpeg", "image/jpg":
		return "jpg"
	case "image/png":
		return "png"
	case "image/gif":
		return "gif"
	case "image/webp":
		return "webp"
	case "application/pdf":
		return "pdf"
	case "application/zip":
		return "zip"
	case "application/octet-stream":
		// 从 fileKey 推断
		if parts := strings.Split(fileKey, "."); len(parts) > 1 {
			return parts[len(parts)-1]
		}
		return "bin"
	default:
		if idx := strings.LastIndex(mimeType, "/"); idx >= 0 {
			return mimeType[idx+1:]
		}
		return "bin"
	}
}
