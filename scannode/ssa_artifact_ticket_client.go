package scannode

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/utils"
)

type ssaArtifactTicketRequest struct {
	TaskID    string `json:"task_id"`
	ObjectKey string `json:"object_key"`
}

type ssaArtifactTicketResponse struct {
	TaskID          string `json:"task_id"`
	ObjectKey       string `json:"object_key"`
	Codec           string `json:"codec"`
	Bucket          string `json:"bucket"`
	Region          string `json:"region"`
	Endpoint        string `json:"endpoint"`
	UseSSL          bool   `json:"use_ssl"`
	STSAccessKey    string `json:"sts_access_key"`
	STSSecretKey    string `json:"sts_secret_key"`
	STSSessionToken string `json:"sts_session_token"`
	STSExpiresAt    int64  `json:"sts_expires_at"`
}

func (s *ScanNode) fetchSSAArtifactUploadTicket(ctx context.Context, taskID, objectKey string) (*SSAArtifactUploadConfig, error) {
	if s == nil || s.node == nil {
		return nil, utils.Errorf("scannode not ready")
	}
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return nil, utils.Errorf("task id required")
	}

	baseURL := strings.TrimRight(strings.TrimSpace(s.resolvePlatformAPIBaseURL()), "/")
	if baseURL == "" {
		return nil, utils.Errorf("server http url unavailable")
	}
	token := strings.TrimSpace(s.node.GetToken())
	if token == "" {
		return nil, utils.Errorf("node token unavailable")
	}

	rawReq, err := json.Marshal(&ssaArtifactTicketRequest{
		TaskID:    taskID,
		ObjectKey: strings.TrimSpace(objectKey),
	})
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/api/ssa/task/artifact-ticket", bytes.NewReader(rawReq))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+token)
	httpReq.Header.Set("Content-Type", "application/json")

	client := &http.Client{
		Timeout: 20 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, utils.Errorf("fetch upload ticket failed: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	rawBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	var ticket ssaArtifactTicketResponse
	if err := json.Unmarshal(rawBody, &ticket); err != nil {
		return nil, err
	}
	if ticket.ObjectKey == "" || ticket.Endpoint == "" || ticket.Bucket == "" || ticket.STSAccessKey == "" || ticket.STSSecretKey == "" {
		// 兼容被统一包装的 API 响应：{"code":200, "data":{...ticket...}}
		var wrapped struct {
			Data ssaArtifactTicketResponse `json:"data"`
		}
		if err := json.Unmarshal(rawBody, &wrapped); err == nil {
			if wrapped.Data.ObjectKey != "" {
				ticket = wrapped.Data
			}
		}
	}
	cfg := &SSAArtifactUploadConfig{
		ObjectKey:       strings.TrimSpace(ticket.ObjectKey),
		Codec:           strings.TrimSpace(ticket.Codec),
		Endpoint:        strings.TrimSpace(ticket.Endpoint),
		Bucket:          strings.TrimSpace(ticket.Bucket),
		Region:          strings.TrimSpace(ticket.Region),
		UseSSL:          ticket.UseSSL,
		STSAccessKey:    strings.TrimSpace(ticket.STSAccessKey),
		STSSecretKey:    strings.TrimSpace(ticket.STSSecretKey),
		STSSessionToken: strings.TrimSpace(ticket.STSSessionToken),
		STSExpiresAt:    ticket.STSExpiresAt,
	}
	if cfg.Codec == "" {
		cfg.Codec = "zstd"
	}
	if cfg.ObjectKey == "" {
		cfg.ObjectKey = strings.TrimSpace(objectKey)
	}
	if cfg.STSAccessKey == "" || cfg.STSSecretKey == "" {
		return nil, utils.Errorf("invalid upload ticket: missing sts credentials")
	}
	return cfg, nil
}

func (s *ScanNode) resolvePlatformAPIBaseURL() string {
	if s == nil {
		return ""
	}
	client, ok := s.ruleSyncClient.(*RuleSyncClient)
	if !ok || client == nil || client.config == nil {
		return ""
	}
	return strings.TrimSpace(client.config.ServerURL)
}
