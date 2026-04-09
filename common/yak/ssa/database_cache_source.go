package ssa

import (
	"path"
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

// sourceStore only tracks source payload registration and the final flush of
// IrSource rows. Instruction persistence never coordinates source saves.
type sourceStore struct {
	mode        ProgramCacheKind
	programName string
	db          *gorm.DB

	mu        sync.Mutex
	payloads  map[string]*ssadb.IrSource
	persisted map[string]struct{}
}

func newSourceStore(prog *Program, mode ProgramCacheKind, db *gorm.DB) *sourceStore {
	programName := ""
	if prog != nil {
		programName = prog.GetProgramName()
	}
	return &sourceStore{
		mode:        mode,
		programName: programName,
		db:          db,
		payloads:    make(map[string]*ssadb.IrSource),
		persisted:   make(map[string]struct{}),
	}
}

func (s *sourceStore) RegisterEditor(editor *memedit.MemEditor) {
	if s == nil || editor == nil {
		return
	}
	hash := editor.GetIrSourceHash()
	log.Debugf("SaveEditor: program=%s, path=%s, hash=%s", editor.GetProgramName(), editor.GetFolderPath()+editor.GetFilename(), hash)
	s.registerSource(hash, ssadb.MarshalFile(editor, hash))
}

func (s *sourceStore) RegisterFolder(folderPath []string) {
	if s == nil {
		return
	}
	ir := ssadb.MarshalFolder(folderPath)
	s.registerSource(ir.SourceCodeHash, ir)
}

func (s *sourceStore) registerSource(hash string, source *ssadb.IrSource) {
	if s == nil || s.mode != ProgramCacheDBWrite || s.db == nil || hash == "" || source == nil {
		return
	}
	if s.isPersisted(hash) {
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

func (s *sourceStore) isPersisted(hash string) bool {
	if s == nil || hash == "" {
		return false
	}

	s.mu.Lock()
	if _, ok := s.persisted[hash]; ok {
		s.mu.Unlock()
		return true
	}
	db := s.db
	programName := s.programName
	s.mu.Unlock()

	if db == nil || programName == "" {
		return false
	}

	var count int
	if err := db.Model(&ssadb.IrSource{}).
		Where("source_code_hash = ?", hash).
		Where("program_name = ?", programName).
		Count(&count).Error; err != nil {
		log.Warnf("IsExistedSourceCodeHash error: %v", err)
		return false
	}
	if count <= 0 {
		return false
	}
	s.markPersisted(hash)
	return true
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

	sources, hashes := s.collectRegisteredSources()
	if len(sources) == 0 {
		return
	}

	saveErr := utils.GormTransaction(s.db, func(tx *gorm.DB) error {
		for _, source := range sources {
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
	s.markPersisted(hashes...)
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
