package filter

import (
	"encoding/binary"
	"github.com/valyala/bytebufferpool"
	"github.com/yaklang/yaklang/common/cuckoo"
	"github.com/yaklang/yaklang/common/utils"
	"sync"
)

var bufferPool = bytebufferpool.Pool{}

type StringFilter struct {
	sync.Mutex
	container   *cuckoo.Filter
	conf        *Config
	lastUpdated int64
}

func (s *StringFilter) build(str string) []byte {
	buf := bufferPool.Get()
	defer func() {
		bufferPool.Put(buf)
	}()

	if s.conf.TTL > 0 {
		// 如果最后一个元素都是过期的，直接释放掉之前的 container
		now := utils.TimestampMs() / 1000
		if s.lastUpdated != 0 && (now-s.lastUpdated >= s.conf.TTL) {
			s.container = NewDirCuckoo()
		}
		_ = binary.Write(buf, binary.LittleEndian, now/s.conf.TTL)
	}
	_, _ = buf.WriteString(str)
	b := buf.Bytes()
	newB := append(b[:0:0], b...)
	return newB
}

func (s *StringFilter) Exist(str string) bool {
	s.Lock()
	defer s.Unlock()
	return s.container.Lookup(s.build(str))
}

// 返回值是 true，插入成功，false 容器满了, 无法继续添加
func (s *StringFilter) Insert(str string) bool {
	s.Lock()
	defer s.Unlock()
	return s.container.Insert(s.build(str))
}

func NewStringFilter(config *Config, container *cuckoo.Filter) *StringFilter {
	return &StringFilter{
		conf:      config,
		container: container,
	}
}

func NewFilter() *StringFilter {
	filterConfig := NewDefaultConfig()
	filterConfig.CaseSensitive = true
	f := NewStringFilter(filterConfig, NewGenericCuckoo())
	return f
}
