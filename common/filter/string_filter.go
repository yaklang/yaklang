package filter

import (
	"encoding/binary"
	"sync"

	"github.com/valyala/bytebufferpool"
	"github.com/yaklang/yaklang/common/cuckoo"
	"github.com/yaklang/yaklang/common/utils"
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

// NewFilter 创建一个默认的字符串布谷鸟过滤器，布谷鸟过滤器用于判断一个元素是否在一个集合中，它存在极低的假阳性（即说存在的元素实际上不存在），通常这个集合中的元素数量非常大才会使用布谷鸟过滤器。
// Example:
// ```
// f = str.NewFilter()
// f.Insert("hello")
// f.Exist("hello") // true
// ```
func NewFilter() *StringFilter {
	filterConfig := NewDefaultConfig()
	filterConfig.CaseSensitive = true
	f := NewStringFilter(filterConfig, NewGenericCuckoo())
	return f
}

func NewFilterWithSize(entries, total uint) *StringFilter {
	filterConfig := NewDefaultConfig()
	filterConfig.CaseSensitive = true
	f := NewStringFilter(filterConfig, cuckoo.New(
		cuckoo.BucketEntries(entries),
		cuckoo.BucketTotal(total),
		cuckoo.Kicks(300),
	))
	return f
}
