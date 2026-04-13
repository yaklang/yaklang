package ssa

import (
	"path"
	"sort"
	"sync"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
)

func (p *Program) CreateEditor(raw []byte, filepath string, save ...bool) *memedit.MemEditor {
	edit := memedit.NewMemEditorByBytes(raw)
	folder, file := path.Split(filepath)

	edit.SetFolderPath(folder)
	edit.SetFileName(file)
	edit.SetProgramName(p.GetProgramName())
	if len(save) == 0 || save[0] {
		p.SaveEditor(edit)
	}
	return edit
}

func (p *Program) SaveEditor(e *memedit.MemEditor) {
	if p.DatabaseKind == ProgramCacheMemory || p.Cache == nil {
		return
	}
	if p.Cache.sources == nil {
		return
	}
	p.Cache.sources.RegisterEditor(e)
}

func (p *Program) SaveFolder(folderPath []string) {
	if p.DatabaseKind == ProgramCacheMemory || p.Cache == nil {
		return
	}
	if p.Cache.sources == nil {
		return
	}
	p.Cache.sources.RegisterFolder(folderPath)
}

// sourceStore keeps editor/source payloads resident until source rows are
// flushed so async lookups can still resolve hashes before the final DB write.
type sourceStore struct {
	mode        ProgramCacheKind
	programName string
	db          *gorm.DB

	mu           sync.Mutex
	payloads     map[string]*ssadb.IrSource
	persisted    map[string]struct{}
	editors      map[string]*memedit.MemEditor
	editorsByURL map[string]*memedit.MemEditor
	visitedURLs  map[string]*memedit.MemEditor
}

func newSourceStore(prog *Program, mode ProgramCacheKind, db *gorm.DB) *sourceStore {
	programName := ""
	if prog != nil {
		programName = prog.GetProgramName()
	}
	return &sourceStore{
		mode:         mode,
		programName:  programName,
		db:           db,
		payloads:     make(map[string]*ssadb.IrSource),
		persisted:    make(map[string]struct{}),
		editors:      make(map[string]*memedit.MemEditor),
		editorsByURL: make(map[string]*memedit.MemEditor),
		visitedURLs:  make(map[string]*memedit.MemEditor),
	}
}

func (s *sourceStore) RegisterEditor(editor *memedit.MemEditor) {
	if s == nil || editor == nil {
		return
	}
	hash := editor.GetIrSourceHash()
	if hash == "" {
		return
	}

	log.Debugf("SaveEditor: program=%s, path=%s, hash=%s", editor.GetProgramName(), editor.GetFolderPath()+editor.GetFilename(), hash)
	s.rememberEditor(editor.GetUrl(), editor)
	s.registerSource(hash, ssadb.MarshalFile(editor, hash))
}

func (s *sourceStore) rememberEditor(url string, editor *memedit.MemEditor) {
	if s == nil || editor == nil {
		return
	}
	hash := editor.GetIrSourceHash()
	if hash == "" {
		return
	}
	if url == "" {
		url = editor.GetUrl()
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.editors[hash] = editor
	if url != "" {
		s.editorsByURL[url] = editor
	}
}

func (s *sourceStore) markVisitedEditor(url string, editor *memedit.MemEditor) {
	if s == nil || editor == nil {
		return
	}
	if url == "" {
		url = editor.GetUrl()
	}
	s.rememberEditor(url, editor)

	s.mu.Lock()
	defer s.mu.Unlock()
	if url != "" {
		s.visitedURLs[url] = editor
	}
}

func (s *sourceStore) RegisterFolder(folderPath []string) {
	if s == nil {
		return
	}
	ir := ssadb.MarshalFolder(folderPath)
	if ir == nil {
		return
	}
	s.registerSource(ir.SourceCodeHash, ir)
}

func (s *sourceStore) registerSource(hash string, source *ssadb.IrSource) {
	if s == nil || s.mode != ProgramCacheDBWrite || s.db == nil || hash == "" || source == nil {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.persisted[hash]; ok {
		return
	}
	if _, ok := s.payloads[hash]; ok {
		return
	}
	s.payloads[hash] = source
}

func (s *sourceStore) markPersisted(hashes ...string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, hash := range hashes {
		if hash == "" {
			continue
		}
		delete(s.payloads, hash)
		s.persisted[hash] = struct{}{}
	}
}

func (s *sourceStore) Close() {
	if s == nil || s.mode != ProgramCacheDBWrite || s.db == nil {
		return
	}
	defer s.releaseEditors()

	sources, hashes := s.collectRegisteredSources()
	if len(sources) == 0 {
		return
	}

	existing, err := s.lookupPersistedHashes(hashes)
	if err != nil {
		log.Errorf("DATABASE: lookup ir source in database error: %v", err)
		return
	}
	if len(existing) > 0 {
		s.markPersisted(existing...)
	}

	existingSet := make(map[string]struct{}, len(existing))
	for _, hash := range existing {
		existingSet[hash] = struct{}{}
	}

	toSave := make([]*ssadb.IrSource, 0, len(sources))
	savedHashes := make([]string, 0, len(sources))
	for idx, source := range sources {
		if source == nil {
			continue
		}
		hash := hashes[idx]
		if _, ok := existingSet[hash]; ok {
			continue
		}
		toSave = append(toSave, source)
		savedHashes = append(savedHashes, hash)
	}
	if len(toSave) == 0 {
		return
	}

	saveErr := utils.GormTransaction(s.db, func(tx *gorm.DB) error {
		for _, source := range toSave {
			if source == nil {
				continue
			}
			if err := tx.Save(source).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if saveErr != nil {
		log.Errorf("DATABASE: save ir source to database error: %v", saveErr)
		return
	}
	s.markPersisted(savedHashes...)
}

func (s *sourceStore) collectRegisteredSources() ([]*ssadb.IrSource, []string) {
	if s == nil {
		return nil, nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	sources := make([]*ssadb.IrSource, 0, len(s.payloads))
	hashes := make([]string, 0, len(s.payloads))
	for hash, source := range s.payloads {
		if hash == "" || source == nil {
			continue
		}
		if _, ok := s.persisted[hash]; ok {
			continue
		}
		sources = append(sources, source)
		hashes = append(hashes, hash)
	}
	return sources, hashes
}

func (s *sourceStore) lookupPersistedHashes(hashes []string) ([]string, error) {
	if s == nil || s.db == nil || len(hashes) == 0 {
		return nil, nil
	}

	var existing []string
	query := s.db.Model(&ssadb.IrSource{}).
		Where("program_name = ?", s.programName).
		Where("source_code_hash IN (?)", hashes)
	if err := query.Pluck("source_code_hash", &existing).Error; err != nil {
		return nil, err
	}
	return existing, nil
}

func (s *sourceStore) releaseEditors() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	clear(s.editors)
	clear(s.editorsByURL)
	clear(s.visitedURLs)
}

func (s *sourceStore) PersistedCount() int {
	if s == nil {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.persisted)
}

func (s *sourceStore) PayloadCount() int {
	if s == nil {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.payloads)
}

func (s *sourceStore) EditorCount() int {
	if s == nil {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.editors)
}

func (s *sourceStore) GetEditorByHash(hash string) (*memedit.MemEditor, bool) {
	if s == nil || hash == "" {
		return nil, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	editor, ok := s.editors[hash]
	return editor, ok
}

func (s *sourceStore) GetEditorByURL(url string) (*memedit.MemEditor, bool) {
	if s == nil {
		return nil, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	editor, ok := s.editorsByURL[url]
	return editor, ok
}

func (s *sourceStore) GetVisitedEditorByURL(url string) (*memedit.MemEditor, bool) {
	if s == nil {
		return nil, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	editor, ok := s.visitedURLs[url]
	return editor, ok
}

func (s *sourceStore) HasEditorURL(url string) bool {
	if s == nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.editorsByURL[url]
	return ok
}

func (s *sourceStore) HasVisitedURL(url string) bool {
	if s == nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.visitedURLs[url]
	return ok
}

func (s *sourceStore) EditorURLs() []string {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	ret := make([]string, 0, len(s.visitedURLs))
	for url := range s.visitedURLs {
		ret = append(ret, url)
	}
	sort.Strings(ret)
	return ret
}
