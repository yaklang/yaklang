package ssadb

import (
	"regexp"
	"strings"
	"sync"

	"github.com/gobwas/glob"
	"github.com/yaklang/yaklang/common/utils"
)

type NameCache struct {
	strings *utils.SafeMapWithKey[string, string]
	sync.RWMutex
	nameToId map[string]int64
	idToName map[int64]string
	program  string
	loaded   bool
	// useDB controls whether the cache touches the SSA database. When false
	// (in-memory programs, e.g. ssa2llvm AOT compiles), the cache never calls
	// GetDB and assigns ids from an in-memory counter — so building the SSA
	// program does not open/migrate the YakIT/SSA database. When true
	// (DB-backed programs, e.g. the interpreter's incremental compile), the
	// cache preloads existing name<->id mappings from the DB and persists new
	// ids via FirstOrCreate, as before.
	useDB  bool
	nextID int64
}

// NewNameCache creates a name cache for program. useDB selects DB-backed mode;
// pass false for in-memory programs so the cache never touches the SSA database.
func NewNameCache(program string, useDB bool) *NameCache {
	cache := &NameCache{
		strings:  utils.NewSafeMapWithKey[string, string](),
		nameToId: make(map[string]int64),
		idToName: make(map[int64]string),
		program:  program,
		useDB:    useDB,
	}
	if useDB {
		cache.Preload()
	}
	return cache
}

// Intern returns the single canonical copy of name that this cache keeps.
//
// It is string interning, not name->id resolution: the map's key and value are
// always the same string, so a lookup result is exactly the canonical copy. Two
// reasons to route names through it:
//
//  1. Memory: IR names arrive as substrings of a source buffer or JSON blob.
//     Holding a substring keeps the whole backing array alive, so the first
//     occurrence is cloned (strings.Clone) and every later caller receives that
//     owned copy instead of pinning a large buffer.
//  2. Identity: repeated names collapse to one allocation, which makes name
//     comparisons and dedup cheap for the millions of IR records a large
//     project produces.
//
// The DB-backed name->id pool is a separate concern (Preload/GetNameID); Intern
// never assigns an id or runs SQL, so it is safe in the in-memory (useDB=false)
// mode too.
func (c *NameCache) Intern(name string) string {
	if c == nil || name == "" {
		return name
	}
	if canonical, exists := c.strings.Get(name); exists {
		return canonical
	}
	owned := strings.Clone(name)
	canonical, _ := c.strings.GetOrLoad(owned, func() (string, error) {
		return owned, nil
	})
	return canonical
}

func (c *NameCache) Preload() {
	if !c.useDB {
		return
	}

	c.RLock()
	if c.loaded {
		c.RUnlock()
		return
	}
	c.RUnlock()

	db := GetDB()
	if db == nil {
		return
	}

	var items []IrNamePool
	if err := db.Where("program_name = ?", c.program).Find(&items).Error; err != nil {
		return
	}

	c.Lock()
	defer c.Unlock()
	if c.loaded {
		return
	}
	for _, item := range items {
		c.nameToId[item.Name] = item.NameID
		c.idToName[item.NameID] = item.Name
	}
	// 约定：name_id=0 表示空字符串 ""
	c.nameToId[""] = 0
	c.idToName[0] = ""
	c.loaded = true
}

func (c *NameCache) GetID(name string) int64 {
	name = c.Intern(name)
	// id=0 约定表示空字符串 ""，数据库 namepool 表中 id=0 对应 name=""
	if name == "" {
		c.Lock()
		c.nameToId[""] = 0
		c.idToName[0] = ""
		c.Unlock()
		return 0
	}

	c.RLock()
	if id, ok := c.nameToId[name]; ok {
		c.RUnlock()
		return id
	}
	c.RUnlock()

	c.Lock()
	defer c.Unlock()

	if id, ok := c.nameToId[name]; ok {
		return id
	}

	if !c.useDB {
		// In-memory: assign a fresh local id. 0 is reserved for "".
		c.nextID++
		id := c.nextID
		c.nameToId[name] = id
		c.idToName[id] = name
		return id
	}

	db := GetDB()
	if db == nil {
		return 0
	}

	entry := IrNamePool{
		ProgramName: c.program,
		Name:        name,
	}

	// This write must not race the asynchronous SSA cache saver transaction.
	// Keep the lookup and insert in one critical section as FirstOrCreate may
	// write after first observing that the name is absent.
	if err := WithSQLiteWriteLock(db.Dialect().GetName(), func() error {
		return db.Where(IrNamePool{ProgramName: c.program, Name: name}).FirstOrCreate(&entry).Error
	}); err != nil {
		return 0
	}

	c.nameToId[name] = entry.NameID
	c.idToName[entry.NameID] = name

	return entry.NameID
}

func (c *NameCache) GetName(id int64) string {
	if id == 0 {
		return ""
	}

	c.RLock()
	if name, ok := c.idToName[id]; ok {
		c.RUnlock()
		return name
	}
	c.RUnlock()

	c.Lock()
	defer c.Unlock()

	if name, ok := c.idToName[id]; ok {
		return name
	}

	if !c.useDB {
		// In-memory: an id not in the map was never assigned by this cache.
		return ""
	}

	db := GetDB()
	if db == nil {
		return ""
	}

	var entry IrNamePool
	if err := db.Where("name_id = ?", id).First(&entry).Error; err != nil {
		return ""
	}

	c.nameToId[entry.Name] = entry.NameID
	c.idToName[entry.NameID] = entry.Name
	return entry.Name
}

func (c *NameCache) GetIDsByPattern(pattern string, mode CompareMode) []int64 {
	c.Preload()

	c.RLock()
	defer c.RUnlock()

	var result []int64

	switch mode {
	case ExactCompare:
		if id, ok := c.nameToId[pattern]; ok {
			result = append(result, id)
		}
	case GlobCompare:
		g, err := glob.Compile(pattern)
		if err != nil {
			return nil
		}
		for name, id := range c.nameToId {
			if g.Match(name) {
				result = append(result, id)
			}
		}
	case RegexpCompare:
		re, err := regexp.Compile(pattern)
		if err != nil {
			return nil
		}
		for name, id := range c.nameToId {
			if re.MatchString(name) {
				result = append(result, id)
			}
		}
	}
	return result
}
