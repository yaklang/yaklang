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
	initOnce sync.Once
}

func NewNameCache() *NameCache {
	return &NameCache{
		nameToId: make(map[string]int64),
		idToName: make(map[int64]string),
	}
}

func (c *NameCache) Preload(program string) {
	c.initOnce.Do(func() {
		db := GetDB()
		if db == nil {
			return
		}

		var items []IrNamePool
		if err := db.Where("program_name = ?", program).Find(&items).Error; err != nil {
			return
		}

		c.Lock()
		defer c.Unlock()
		for _, item := range items {
			c.nameToId[item.Name] = item.NameID
			c.idToName[item.NameID] = item.Name
		}
		// 约定：name_id=0 表示空字符串 ""
		c.nameToId[""] = 0
		c.idToName[0] = ""
	})
}

func (c *NameCache) GetID(program, name string) int64 {
	// id=0 约定表示空字符串 ""，数据库 namepool 表中 id=0 对应 name=""
	if name == "" {
		c.Lock()
		c.nameToId[""] = 0
		c.idToName[0] = ""
		db := GetDB()
		if db != nil {
			// 确保 namepool 表中存在 (program_name, name_id=0, name="") 的记录
			var n int64
			if db.Model(&IrNamePool{}).Where("program_name = ? AND name = ?", program, "").Count(&n).Error == nil && n == 0 {
				db.Exec("INSERT INTO ir_name_pool (name_id, program_name, name) VALUES (0, ?, ?)", program, "")
			}
		}
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
		ProgramName: program,
		Name:        name,
	}

	if err := db.Where(IrNamePool{ProgramName: program, Name: name}).FirstOrCreate(&entry).Error; err != nil {
		return 0
	}

	c.nameToId[name] = entry.NameID
	c.idToName[entry.NameID] = name

	return entry.NameID
}

func (c *NameCache) GetName(program string, id int64) string {
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

func (c *NameCache) GetIDsByPattern(program string, pattern string, mode CompareMode) []int64 {
	c.Preload(program)

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
