package core

import (
	"encoding/binary"
	"sync"
	"github.com/yaklang/yaklang/common/cuckoo"
	"github.com/yaklang/yaklang/common/filter"
	"github.com/yaklang/yaklang/common/utils"

	"github.com/valyala/bytebufferpool"
)

var bufferPool = bytebufferpool.Pool{}

type StringFilterwithCount struct {
	sync.Mutex
	container   *cuckoo.Filter
	conf        *filter.Config
	lastUpdated int64
	count       int64
}

func (s *StringFilterwithCount) build(str string) []byte {
	buf := bufferPool.Get()
	defer func() {
		bufferPool.Put(buf)
	}()

	if s.conf.TTL > 0 {
		// 如果最后一个元素都是过期的，直接释放掉之前的 container
		now := utils.TimestampMs() / 1000
		if s.lastUpdated != 0 && (now-s.lastUpdated >= s.conf.TTL) {
			s.container = filter.NewDirCuckoo()
		}
		_ = binary.Write(buf, binary.LittleEndian, now/s.conf.TTL)
	}
	_, _ = buf.WriteString(str)
	b := buf.Bytes()
	newB := append(b[:0:0], b...)
	return newB
}

func (s *StringFilterwithCount) Exist(str string) bool {
	s.Lock()
	defer s.Unlock()
	return s.container.Lookup(s.build(str))
}

// 返回值是 true，插入成功，false 容器满了, 无法继续添加
func (s *StringFilterwithCount) Insert(str string) bool {
	s.Lock()
	defer s.Unlock()
	status := s.container.Insert(s.build(str))
	if status {
		s.count++
	}
	return status
}

func (s *StringFilterwithCount) Count() int64 {
	return s.count
}

func NewStringFilterwithCount(config *filter.Config, container *cuckoo.Filter) *StringFilterwithCount {
	return &StringFilterwithCount{
		conf:      config,
		container: container,
	}
}

func NewFilterwithCount() *StringFilterwithCount {
	filterConfig := filter.NewDefaultConfig()
	filterConfig.CaseSensitive = true
	f := NewStringFilterwithCount(filterConfig, filter.NewGenericCuckoo())
	return f
}
