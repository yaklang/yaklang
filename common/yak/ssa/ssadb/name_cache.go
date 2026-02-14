package ssadb

import (
	"regexp"
	"sync"

	"github.com/gobwas/glob"
)

type NameCache struct {
	sync.RWMutex
	nameToId map[string]int64
	idToName map[int64]string
	program  string
	loaded   bool
}

func NewNameCache(program string) *NameCache {
	cache := &NameCache{
		nameToId: make(map[string]int64),
		idToName: make(map[int64]string),
		program:  program,
	}
	cache.Preload()
	return cache
}

func (c *NameCache) Preload() {
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

	db := GetDB()
	if db == nil {
		return 0
	}

	entry := IrNamePool{
		ProgramName: c.program,
		Name:        name,
	}

	if err := db.Where(IrNamePool{ProgramName: c.program, Name: name}).FirstOrCreate(&entry).Error; err != nil {
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
