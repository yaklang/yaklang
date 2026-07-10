package dingtalk

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	neturl "net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/notify"
	"github.com/yaklang/yaklang/common/notify/internal/httpclient"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
)

const maxDingTalkAttachmentBytes = 25 * 1024 * 1024

var allowedDingTalkImageMIME = map[string]bool{
	"image/jpeg": true,
	"image/png":  true,
	"image/webp": true,
	"image/gif":  true,
}

func (c *Client) DownloadResource(config *notify.SendConfig, downloadCode string, isImage bool) (localPath, mimeType string, size int64, err error) {
	downloadCode = strings.TrimSpace(downloadCode)
	if downloadCode == "" {
		return "", "", 0, fmt.Errorf("dingtalk: downloadCode is required")
	}
	cfg := c.effectiveConfig(config)
	downloadURL, err := c.getDownloadURL(cfg, downloadCode)
	if err != nil {
		return "", "", 0, err
	}

	baseURL, query := splitURLQuery(downloadURL)
	raw, err := httpclient.Request("GET", baseURL, map[string]string{}, query, nil, buildHTTPOpts(cfg.Proxy, cfg.Timeout)...)
	if err != nil {
		return "", "", 0, fmt.Errorf("dingtalk: download media: %w", err)
	}
	statusCode := lowhttp.GetStatusCodeFromResponse(raw)
	if statusCode != 200 {
		body := lowhttp.GetHTTPPacketBody(raw)
		return "", "", 0, fmt.Errorf("dingtalk: media download status %d: %s", statusCode, string(body))
	}

	body := lowhttp.GetHTTPPacketBody(raw)
	mimeType = normalizeMIME(lowhttp.GetHTTPPacketHeader(raw, "Content-Type"))
	if mimeType == "" || mimeType == "application/octet-stream" {
		mimeType = normalizeMIME(http.DetectContentType(body))
	}
	if strings.HasPrefix(mimeType, "application/json") {
		return "", "", 0, fmt.Errorf("dingtalk: media download returned JSON error: %s", string(body))
	}
	if isImage && !allowedDingTalkImageMIME[mimeType] {
		return "", "", 0, fmt.Errorf("dingtalk: image mime %q not in whitelist", mimeType)
	}
	size = int64(len(body))
	if size > maxDingTalkAttachmentBytes {
		return "", "", 0, fmt.Errorf("dingtalk: attachment too large: %d bytes (max %d)", size, maxDingTalkAttachmentBytes)
	}

	mediaDir := consts.TempAIDir("im-media")
	ext := dingtalkMimeToExt(mimeType, isImage)
	randBytes := make([]byte, 8)
	_, _ = rand.Read(randBytes)
	fileName := fmt.Sprintf("%s.%s", hex.EncodeToString(randBytes), ext)
	localPath = filepath.Join(mediaDir, fileName)
	if err := os.WriteFile(localPath, body, 0644); err != nil {
		return "", "", 0, fmt.Errorf("dingtalk: write media file: %w", err)
	}

	log.Debugf("dingtalk: downloaded media to %s (%d bytes, mime=%s)", localPath, size, mimeType)
	return localPath, mimeType, size, nil
}

func (c *Client) getDownloadURL(cfg *notify.SendConfig, downloadCode string) (string, error) {
	if cfg == nil {
		return "", fmt.Errorf("dingtalk: config is required for download")
	}
	if strings.TrimSpace(cfg.AppID) == "" {
		return "", fmt.Errorf("dingtalk: robotCode/appKey is required for download")
	}
	token, err := c.tokens.getToken()
	if err != nil {
		return "", fmt.Errorf("dingtalk: get token for media download: %w", err)
	}
	headers := jsonHeaders()
	headers["x-acs-dingtalk-access-token"] = token
	url := c.base() + "/v1.0/robot/messageFiles/download"
	result, err := httpclient.Do("POST", url, headers, nil, map[string]string{
		"downloadCode": downloadCode,
		"robotCode":    cfg.AppID,
	}, buildHTTPOpts(cfg.Proxy, cfg.Timeout)...)
	if err != nil {
		return "", fmt.Errorf("dingtalk: get media download url: %w", err)
	}
	if result.StatusCode != 200 {
		return "", fmt.Errorf("dingtalk: get media download url status %d: %s", result.StatusCode, string(result.Body))
	}
	var resp struct {
		DownloadURL string `json:"downloadUrl"`
	}
	if err := json.Unmarshal(result.Body, &resp); err != nil {
		return "", fmt.Errorf("dingtalk: decode media download url: %w", err)
	}
	if resp.DownloadURL == "" {
		var fallback map[string]any
		_ = json.Unmarshal(result.Body, &fallback)
		if s, _ := fallback["downloadURL"].(string); s != "" {
			resp.DownloadURL = s
		}
	}
	if resp.DownloadURL == "" {
		return "", fmt.Errorf("dingtalk: empty media download url: %s", string(result.Body))
	}
	return resp.DownloadURL, nil
}

func splitURLQuery(rawURL string) (string, map[string]string) {
	u, err := neturl.Parse(rawURL)
	if err != nil || u.RawQuery == "" {
		return rawURL, nil
	}
	query := map[string]string{}
	for k, vs := range u.Query() {
		if len(vs) > 0 {
			query[k] = vs[0]
		}
	}
	u.RawQuery = ""
	return u.String(), query
}

func normalizeMIME(value string) string {
	value = strings.TrimSpace(value)
	if idx := strings.Index(value, ";"); idx >= 0 {
		value = strings.TrimSpace(value[:idx])
	}
	return strings.ToLower(value)
}

func dingtalkMimeToExt(mimeType string, isImage bool) string {
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
		if isImage {
			return "jpg"
		}
		return "bin"
	default:
		if idx := strings.LastIndex(mimeType, "/"); idx >= 0 {
			return mimeType[idx+1:]
		}
		return "bin"
	}
}
