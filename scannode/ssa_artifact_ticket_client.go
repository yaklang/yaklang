package scannode

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

type ssaArtifactTicketRequest struct {
	TaskID    string `json:"task_id"`
	ObjectKey string `json:"object_key"`
}

type ssaArtifactTicketResponse struct {
	TaskID           string      `json:"task_id"`
	ObjectKey        string      `json:"object_key"`
	Codec            string      `json:"codec"`
	Bucket           string      `json:"bucket"`
	Region           string      `json:"region"`
	Endpoint         string      `json:"endpoint"`
	UseSSL           bool        `json:"use_ssl"`
	TLSVerify        bool        `json:"tls_verify"`
	AllowInsecureTLS bool        `json:"allow_insecure_tls"`
	TLSCAFile        string      `json:"tls_ca_file"`
	AllowHTTP        bool        `json:"allow_http"`
	VirtualHostStyle bool        `json:"virtual_host_style"`
	STSAccessKey     secretValue `json:"sts_access_key"`
	STSSecretKey     secretValue `json:"sts_secret_key"`
	STSSessionToken  secretValue `json:"sts_session_token"`
	STSExpiresAt     int64       `json:"sts_expires_at"`
}

func (s *ScanNode) fetchSSAArtifactUploadTicket(ctx context.Context, taskID, objectKey string) (cfg *SSAArtifactUploadConfig, err error) {
	fetchStart := time.Now()
	defer func() {
		fetchMs := time.Since(fetchStart).Milliseconds()
		if err != nil {
			log.Warnf("ticket_fetch_failed task=%s duration_ms=%d error=%q", taskID, fetchMs, err.Error())
		} else {
			log.Infof("ticket_fetch_ok task=%s duration_ms=%d", taskID, fetchMs)
		}
	}()
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
	parsedBaseURL, err := url.Parse(baseURL)
	if err != nil || parsedBaseURL.Host == "" || (parsedBaseURL.Scheme != "https" && parsedBaseURL.Scheme != "http") || parsedBaseURL.User != nil {
		return nil, utils.Errorf("invalid server URL")
	}
	if parsedBaseURL.Scheme == "http" && !ssaExplicitHTTPAllowed() {
		return nil, utils.Errorf("plaintext ticket endpoint requires SCANNODE_SSA_ALLOW_HTTP=1")
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

	client, err := newSSATicketHTTPClient()
	if err != nil {
		return nil, err
	}
	defer client.CloseIdleConnections()
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return nil, utils.Errorf("fetch upload ticket failed: status=%d request_id=%s", resp.StatusCode, strings.TrimSpace(resp.Header.Get("x-request-id")))
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
	ticketEndpoint := strings.TrimSpace(ticket.Endpoint)
	legacySchemeLessHTTP := ticketEndpoint != "" && !ticket.UseSSL && !strings.Contains(ticketEndpoint, "://")
	cfg = &SSAArtifactUploadConfig{
		ObjectKey:        strings.TrimSpace(ticket.ObjectKey),
		Codec:            strings.TrimSpace(ticket.Codec),
		Endpoint:         ticketEndpoint,
		Bucket:           strings.TrimSpace(ticket.Bucket),
		Region:           strings.TrimSpace(ticket.Region),
		UseSSL:           ticket.UseSSL,
		TLSVerify:        ticket.TLSVerify,
		AllowInsecureTLS: ticket.AllowInsecureTLS,
		TLSCAFile:        strings.TrimSpace(ticket.TLSCAFile),
		AllowHTTP:        ticket.AllowHTTP || legacySchemeLessHTTP || ssaExplicitHTTPAllowed(),
		VirtualHostStyle: ticket.VirtualHostStyle,
		STSExpiresAt:     ticket.STSExpiresAt,
	}
	cfg.setSTSCredentials(ticket.STSAccessKey.raw(), ticket.STSSecretKey.raw(), ticket.STSSessionToken.raw())
	if cfg.Codec == "" {
		cfg.Codec = "zstd"
	}
	if cfg.ObjectKey == "" {
		cfg.ObjectKey = strings.TrimSpace(objectKey)
	}
	if cfg.accessKeySecret().raw() == "" || cfg.secretKeySecret().raw() == "" {
		return nil, utils.Errorf("invalid upload ticket: missing sts credentials")
	}
	return cfg, nil
}

func newSSATicketHTTPClient() (*http.Client, error) {
	allowInsecureTLS := false
	switch strings.ToLower(strings.TrimSpace(os.Getenv("SCANNODE_SSA_TICKET_ALLOW_INSECURE_TLS"))) {
	case "1", "true", "yes", "on":
		allowInsecureTLS = true
	}
	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12, InsecureSkipVerify: allowInsecureTLS}
	caFile := strings.TrimSpace(os.Getenv("SCANNODE_SSA_TICKET_TLS_CA_FILE"))
	if caFile == "" {
		caFile = strings.TrimSpace(os.Getenv("SCANNODE_SSA_TLS_CA_FILE"))
	}
	if caFile != "" {
		pemBytes, err := readSSACertificateFile(caFile)
		if err != nil {
			return nil, utils.Errorf("read ticket CA file: %v", err)
		}
		pool, err := x509.SystemCertPool()
		if err != nil || pool == nil {
			pool = x509.NewCertPool()
		}
		if !pool.AppendCertsFromPEM(pemBytes) {
			return nil, utils.Errorf("ticket CA file contains no certificates")
		}
		tlsConfig.RootCAs = pool
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = tlsConfig
	return &http.Client{Timeout: 20 * time.Second, Transport: transport}, nil
}

func ssaExplicitHTTPAllowed() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("SCANNODE_SSA_ALLOW_HTTP"))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
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
