package scannode

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/node"
	"github.com/yaklang/yaklang/common/utils"
)

const sourcePayloadDownloadPathPrefix = "/v1/ssa-source-payloads/"

const sourcePayloadDownloadTimeout = 60 * time.Second

var managedSourcePayloadIDPattern = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

// prepareManagedSourcePayload downloads a Legion-managed archive with the
// Scan Node's current runtime session and rewrites CodeSource to use the local
// temporary file. The task-provided URL is deliberately ignored: sending the
// node session token to that URL would allow a forged job input to exfiltrate
// the credential.
func (s *ScanNode) prepareManagedSourcePayload(
	ctx context.Context,
	params map[string]any,
) (func(), error) {
	reference, err := managedCodeSource(params)
	if err != nil {
		return nil, err
	}
	if reference == nil {
		return func() {}, nil
	}
	if s == nil || s.node == nil {
		return nil, utils.Errorf("scan node is not ready for source payload download")
	}
	session, ok := s.node.GetSessionState()
	if !ok {
		return nil, utils.Errorf("node session is unavailable for source payload download")
	}

	return downloadAndRewriteManagedSourcePayload(
		ctx,
		s.httpClient,
		s.resolvePlatformAPIBaseURL(),
		session,
		reference,
	)
}

type managedCodeSourceReference struct {
	codeSource map[string]any
	payloadID  string
	persist    func() error
}

func managedCodeSource(params map[string]any) (*managedCodeSourceReference, error) {
	if params == nil {
		return nil, nil
	}
	if reference := managedCodeSourceFromRoot(params); reference != nil {
		reference.persist = func() error { return nil }
		return reference, nil
	}

	configJSON, ok := params["config"].(string)
	if !ok || strings.TrimSpace(configJSON) == "" {
		return nil, nil
	}
	configRoot := make(map[string]any)
	if err := json.Unmarshal([]byte(configJSON), &configRoot); err != nil {
		return nil, nil
	}
	reference := managedCodeSourceFromRoot(configRoot)
	if reference == nil {
		return nil, nil
	}
	reference.persist = func() error {
		raw, err := json.Marshal(configRoot)
		if err != nil {
			return utils.Errorf("marshal managed source payload config: %v", err)
		}
		params["config"] = string(raw)
		return nil
	}
	return reference, nil
}

func managedCodeSourceFromRoot(root map[string]any) *managedCodeSourceReference {
	codeSource, ok := root["CodeSource"].(map[string]any)
	if !ok || codeSource == nil {
		return nil
	}
	kind, _ := codeSource["kind"].(string)
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "compression", "jar":
	default:
		return nil
	}
	payloadID, ok := codeSource["payload_id"].(string)
	payloadID = strings.TrimSpace(payloadID)
	if !ok || payloadID == "" {
		return nil
	}
	return &managedCodeSourceReference{codeSource: codeSource, payloadID: payloadID}
}

func downloadAndRewriteManagedSourcePayload(
	ctx context.Context,
	client *http.Client,
	platformAPIBaseURL string,
	session node.SessionState,
	reference *managedCodeSourceReference,
) (func(), error) {
	localFile, err := downloadManagedSourcePayload(ctx, client, platformAPIBaseURL, session, reference.payloadID)
	if err != nil {
		return nil, err
	}
	reference.codeSource["local_file"] = localFile
	delete(reference.codeSource, "url")
	if err := reference.persist(); err != nil {
		_ = os.Remove(localFile)
		return nil, err
	}
	return func() { _ = os.Remove(localFile) }, nil
}

func downloadManagedSourcePayload(
	ctx context.Context,
	client *http.Client,
	platformAPIBaseURL string,
	session node.SessionState,
	payloadID string,
) (localFile string, err error) {
	localFile, _, err = downloadManagedSourcePayloadBounded(
		ctx,
		client,
		platformAPIBaseURL,
		session.SessionID,
		session.SessionToken,
		payloadID,
		0,
		"",
	)
	return localFile, err
}

func downloadManagedSourcePayloadBounded(
	ctx context.Context,
	client *http.Client,
	platformAPIBaseURL string,
	nodeSessionID string,
	nodeSessionToken string,
	payloadID string,
	maxBytes int64,
	expectedSHA256 string,
) (localFile string, actualSHA256 string, err error) {
	payloadID = strings.TrimSpace(payloadID)
	if !managedSourcePayloadIDPattern.MatchString(payloadID) {
		return "", "", utils.Errorf("invalid managed source payload id")
	}
	if strings.TrimSpace(nodeSessionID) == "" || strings.TrimSpace(nodeSessionToken) == "" {
		return "", "", utils.Errorf("node session credentials are unavailable for source payload download")
	}

	baseURL, err := url.Parse(strings.TrimSpace(platformAPIBaseURL))
	if err != nil ||
		baseURL.Scheme == "" ||
		baseURL.Host == "" ||
		(baseURL.Scheme != "http" && baseURL.Scheme != "https") ||
		baseURL.User != nil ||
		baseURL.RawQuery != "" ||
		baseURL.Fragment != "" {
		return "", "", utils.Errorf("invalid platform API base URL for source payload download")
	}
	baseURL.Path = strings.TrimRight(baseURL.Path, "/") + sourcePayloadDownloadPathPrefix + url.PathEscape(payloadID) + "/download"
	baseURL.RawPath = ""
	query := baseURL.Query()
	query.Set("node_session_id", strings.TrimSpace(nodeSessionID))
	baseURL.RawQuery = query.Encode()

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL.String(), nil)
	if err != nil {
		return "", "", utils.Errorf("build source payload download request: %v", err)
	}
	request.Header.Set("Authorization", "Bearer "+strings.TrimSpace(nodeSessionToken))

	httpClient := client
	if httpClient == nil {
		httpClient = &http.Client{Timeout: sourcePayloadDownloadTimeout}
	}
	requestClient := *httpClient
	if requestClient.Timeout <= 0 {
		requestClient.Timeout = sourcePayloadDownloadTimeout
	}
	// Never forward the node session credential through an HTTP redirect. The
	// configured Legion endpoint streams the payload directly; a redirect is an
	// unexpected response and must fail closed.
	requestClient.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}
	response, err := requestClient.Do(request)
	if err != nil {
		return "", "", utils.Errorf("download managed source payload: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", "", utils.Errorf("download managed source payload failed: status=%d", response.StatusCode)
	}
	if maxBytes > 0 && response.ContentLength > maxBytes {
		return "", "", utils.Errorf(
			"download managed source payload exceeds max bytes: content_length=%d max_bytes=%d",
			response.ContentLength,
			maxBytes,
		)
	}

	tempFile, err := os.CreateTemp("", "yaklang-node-source-payload-*.zip")
	if err != nil {
		return "", "", utils.Errorf("create source payload temp file: %v", err)
	}
	tempPath := tempFile.Name()
	defer func() {
		closeErr := tempFile.Close()
		if err == nil && closeErr != nil {
			err = utils.Errorf("close source payload temp file: %v", closeErr)
		}
		if err != nil {
			_ = os.Remove(tempPath)
		}
	}()
	hash := sha256.New()
	reader := io.Reader(response.Body)
	if maxBytes > 0 {
		reader = io.LimitReader(response.Body, maxBytes+1)
	}
	written, copyErr := io.Copy(io.MultiWriter(tempFile, hash), reader)
	if copyErr != nil {
		return "", "", utils.Errorf("write source payload temp file: %v", copyErr)
	}
	if maxBytes > 0 && written > maxBytes {
		return "", "", utils.Errorf("download managed source payload exceeds max bytes: max_bytes=%d", maxBytes)
	}
	if err = tempFile.Sync(); err != nil {
		return "", "", utils.Errorf("sync source payload temp file: %v", err)
	}
	actualSHA256 = hex.EncodeToString(hash.Sum(nil))
	if expected := strings.ToLower(strings.TrimSpace(expectedSHA256)); expected != "" && expected != actualSHA256 {
		return "", "", utils.Errorf(
			"download managed source payload sha256 mismatch: expected=%s actual=%s",
			expected,
			actualSHA256,
		)
	}
	return tempPath, actualSHA256, nil
}
