package yakurl

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/openapi"
	"github.com/yaklang/yaklang/common/utils"
)

const (
	openAPIDocumentSessionFile      = "session.json"
	openAPIDocumentLegacyMetaFile   = "meta.json"
	openAPIDocumentSource           = "openapi-doc"
)

// openAPIDocumentSession mirrors AISession metadata fields for consistent history UI.
type openAPIDocumentSession struct {
	SessionID  string `json:"session_id"`
	Title      string `json:"title"`
	FileName   string `json:"file_name,omitempty"`
	Source     string `json:"source"`
	CreatedAt  int64  `json:"created_at"`
	UpdatedAt  int64  `json:"updated_at"`
	LastUsedAt int64  `json:"last_used_at"`
	SpecFile   string `json:"spec_file"`
}

type openAPIDocumentLegacyMeta struct {
	UploadedAt int64  `json:"uploaded_at"`
	FileName   string `json:"file_name,omitempty"`
	SpecFile   string `json:"spec_file"`
}

var openAPIDocumentStoreLoadOnce sync.Once

func ensureOpenAPIDocumentStoreLoaded() {
	openAPIDocumentStoreLoadOnce.Do(func() {
		if err := loadOpenAPIDocumentsFromDisk(); err != nil {
			log.Warnf("load openapi documents from disk failed: %v", err)
		}
	})
}

// ResetOpenAPIDocumentStoreForTest clears the in-memory cache and reload flag.
func ResetOpenAPIDocumentStoreForTest() {
	openAPIDocumentStore.Range(func(key, value any) bool {
		openAPIDocumentStore.Delete(key)
		return true
	})
	openAPIDocumentStoreLoadOnce = sync.Once{}
}

func openAPIDocumentBaseDir() string {
	return consts.GetDefaultYakitOpenAPIDocumentsDir()
}

func openAPIDocumentDocDir(docID string) (string, error) {
	if err := validateOpenAPIDocumentID(docID); err != nil {
		return "", err
	}
	return filepath.Join(openAPIDocumentBaseDir(), docID), nil
}

func validateOpenAPIDocumentID(docID string) error {
	if _, err := uuid.Parse(docID); err != nil {
		return utils.Errorf("invalid openapi document id: %q", docID)
	}
	return nil
}

func openAPISpecFileName(fileName, content string) string {
	if name := sanitizeOpenAPISpecFileName(fileName); name != "" {
		return name
	}
	trimmed := strings.TrimSpace(content)
	if strings.HasPrefix(trimmed, "openapi:") || strings.Contains(trimmed, "\nopenapi:") {
		return "spec.yaml"
	}
	return "spec.json"
}

func sanitizeOpenAPISpecFileName(fileName string) string {
	fileName = strings.TrimSpace(fileName)
	if fileName == "" {
		return ""
	}
	base := filepath.Base(filepath.FromSlash(fileName))
	if base == "." || base == string(filepath.Separator) {
		return ""
	}
	ext := strings.ToLower(filepath.Ext(base))
	switch ext {
	case ".json", ".yaml", ".yml":
		return base
	default:
		return base + ".json"
	}
}

func newOpenAPIDocumentSession(docID, title, fileName, specFile string, now int64) openAPIDocumentSession {
	title = strings.TrimSpace(title)
	if title == "" {
		title = docID
	}
	return openAPIDocumentSession{
		SessionID:  docID,
		Title:      title,
		FileName:   strings.TrimSpace(fileName),
		Source:     openAPIDocumentSource,
		CreatedAt:  now,
		UpdatedAt:  now,
		LastUsedAt: now,
		SpecFile:   specFile,
	}
}

func (s *openAPIDocumentSession) touchLastUsed() {
	now := time.Now().Unix()
	s.LastUsedAt = now
	s.UpdatedAt = now
}

func saveOpenAPIDocumentToDisk(docID string, doc *cachedOpenAPIDocument) error {
	if doc == nil {
		return utils.Error("openapi document is nil")
	}
	docDir, err := openAPIDocumentDocDir(docID)
	if err != nil {
		return err
	}
	specFile := doc.Session.SpecFile
	if specFile == "" {
		specFile = openAPISpecFileName(doc.Session.FileName, doc.Content)
		doc.Session.SpecFile = specFile
	}
	if err := os.MkdirAll(docDir, 0o755); err != nil {
		return utils.Wrap(err, "create openapi document dir")
	}
	specPath := filepath.Join(docDir, specFile)
	if err := os.WriteFile(specPath, []byte(doc.Content), 0o644); err != nil {
		return utils.Wrap(err, "write openapi spec file")
	}
	if doc.Session.SessionID == "" {
		doc.Session.SessionID = docID
	}
	if doc.Session.Source == "" {
		doc.Session.Source = openAPIDocumentSource
	}
	sessionRaw, err := json.Marshal(doc.Session)
	if err != nil {
		return utils.Wrap(err, "marshal openapi document session")
	}
	sessionPath := filepath.Join(docDir, openAPIDocumentSessionFile)
	if err := os.WriteFile(sessionPath, sessionRaw, 0o644); err != nil {
		return utils.Wrap(err, "write openapi document session")
	}
	return nil
}

func deleteOpenAPIDocumentFromDisk(docID string) error {
	docDir, err := openAPIDocumentDocDir(docID)
	if err != nil {
		return err
	}
	if err := os.RemoveAll(docDir); err != nil {
		return utils.Wrap(err, "remove openapi document dir")
	}
	return nil
}

func loadOpenAPIDocumentsFromDisk() error {
	baseDir := openAPIDocumentBaseDir()
	entries, err := os.ReadDir(baseDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return utils.Wrap(err, "read openapi documents dir")
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		docID := entry.Name()
		doc, err := loadOpenAPIDocumentFromDisk(docID)
		if err != nil {
			log.Warnf("skip invalid openapi document %q: %v", docID, err)
			continue
		}
		openAPIDocumentStore.Store(docID, doc)
	}
	return nil
}

func readOpenAPIDocumentSession(docDir, docID string) (openAPIDocumentSession, error) {
	sessionPath := filepath.Join(docDir, openAPIDocumentSessionFile)
	if sessionRaw, err := os.ReadFile(sessionPath); err == nil {
		var session openAPIDocumentSession
		if err := json.Unmarshal(sessionRaw, &session); err != nil {
			return openAPIDocumentSession{}, utils.Wrap(err, "parse openapi document session")
		}
		if session.SessionID == "" {
			session.SessionID = docID
		}
		if session.Source == "" {
			session.Source = openAPIDocumentSource
		}
		return session, nil
	} else if !os.IsNotExist(err) {
		return openAPIDocumentSession{}, utils.Wrap(err, "read openapi document session")
	}

	legacyPath := filepath.Join(docDir, openAPIDocumentLegacyMetaFile)
	legacyRaw, err := os.ReadFile(legacyPath)
	if err != nil {
		return openAPIDocumentSession{}, utils.Wrap(err, "read openapi document session")
	}
	var legacy openAPIDocumentLegacyMeta
	if err := json.Unmarshal(legacyRaw, &legacy); err != nil {
		return openAPIDocumentSession{}, utils.Wrap(err, "parse legacy openapi document meta")
	}
	ts := legacy.UploadedAt
	if ts == 0 {
		ts = time.Now().Unix()
	}
	return openAPIDocumentSession{
		SessionID:  docID,
		Title:      docID,
		FileName:   legacy.FileName,
		Source:     openAPIDocumentSource,
		CreatedAt:  ts,
		UpdatedAt:  ts,
		LastUsedAt: ts,
		SpecFile:   legacy.SpecFile,
	}, nil
}

func loadOpenAPIDocumentFromDisk(docID string) (*cachedOpenAPIDocument, error) {
	docDir, err := openAPIDocumentDocDir(docID)
	if err != nil {
		return nil, err
	}
	session, err := readOpenAPIDocumentSession(docDir, docID)
	if err != nil {
		return nil, err
	}
	specFile := strings.TrimSpace(session.SpecFile)
	if specFile == "" {
		specFile = "spec.json"
	}
	specPath := filepath.Join(docDir, filepath.Base(specFile))
	contentRaw, err := os.ReadFile(specPath)
	if err != nil {
		return nil, utils.Wrap(err, "read openapi spec file")
	}
	content := string(contentRaw)
	parsed, err := openapi.ParseDocument(content, nil)
	if err != nil {
		return nil, utils.Wrap(err, "parse openapi document from disk")
	}
	if strings.TrimSpace(session.Title) == "" || session.Title == docID {
		session.Title = strings.TrimSpace(parsed.Info.Title)
		if session.Title == "" {
			session.Title = docID
		}
	}
	return &cachedOpenAPIDocument{
		Content: content,
		Parsed:  parsed,
		Session: session,
	}, nil
}

func storeOpenAPIDocument(docID string, doc *cachedOpenAPIDocument) error {
	openAPIDocumentStore.Store(docID, doc)
	return saveOpenAPIDocumentToDisk(docID, doc)
}

func removeOpenAPIDocument(docID string) error {
	openAPIDocumentStore.Delete(docID)
	return deleteOpenAPIDocumentFromDisk(docID)
}

func touchOpenAPIDocumentLastUsed(docID string, doc *cachedOpenAPIDocument) {
	if doc == nil {
		return
	}
	doc.Session.touchLastUsed()
	openAPIDocumentStore.Store(docID, doc)
	if err := saveOpenAPIDocumentToDisk(docID, doc); err != nil {
		log.Warnf("update openapi document last_used_at failed: %v", err)
	}
}
