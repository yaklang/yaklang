package filter

import (
	"encoding/binary"
	"sort"
	"strconv"
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

// RemoveDuplicatePorts 解析两个字符串形式的端口列表，并使用布谷鸟过滤器进行去重。
// 这个函数首先创建一个布谷鸟过滤器，然后将两个输入字符串解析为端口列表。
// 接着，它遍历这两个列表，将每个端口添加到布谷鸟过滤器中，如果这个端口之前没有被添加过，
// 那么它也会被添加到结果列表中。最后，函数返回结果列表，其中包含两个输入字符串中的所有唯一端口。
// Example:
// ```
// RemoveDuplicatePorts("10086-10088,23333", "10086,10089,23333") // [10086, 10087, 10088, 23333, 10089]
// ```
func RemoveDuplicatePorts(ports1, ports2 string) []int {
	filter := NewFilter()

	parsedPorts1 := utils.ParseStringToPorts(ports1)
	parsedPorts2 := utils.ParseStringToPorts(ports2)

	// 合并并排序 ports1 和 ports2
	allPorts := append(parsedPorts1, parsedPorts2...)
	sort.Ints(allPorts)

	var uniquePorts []int

	// 将唯一的元素添加到结果中
	for _, port := range allPorts {
		if !filter.Exist(strconv.Itoa(port)) {
			filter.Insert(strconv.Itoa(port))
			uniquePorts = append(uniquePorts, port)
		}
	}

	return uniquePorts
}

// FilterPorts 接受两个字符串形式的端口列表作为参数，返回一个新的端口列表，
// 其中包含了在 `ports1` 中但不在 `ports2` 中的所有端口。
// 这个函数首先将两个输入字符串解析为端口列表，然后创建一个映射（或集合）来存储 `ports2` 中的所有端口。
// 然后，它遍历 `ports1` 中的每个端口，如果这个端口不在 `ports2` 中，那么它就会被添加到结果列表中。
// 最后，函数返回结果列表，其中包含了所有只在 `ports1` 中出现的端口。
// Example:
// ```
// FilterPorts("1-10", "2-10") // [1]
// ```
func FilterPorts(sourcePorts, excludePorts string) []int {
	p1 := utils.ParseStringToPorts(sourcePorts)
	p2 := utils.ParseStringToPorts(excludePorts)

	// Create a cuckoo filter for quick lookup of ports in ports2
	f := NewFilter()
	for _, v := range p2 {
		f.Insert(strconv.Itoa(v)) // Convert int to string before inserting
	}

	// Filter ports in ports1
	result := make([]int, 0)
	for _, v := range p1 {
		if !f.Exist(strconv.Itoa(v)) { // Convert int to string before checking
			result = append(result, v)
		}
	}

	return result
}

func (s *StringFilter) Clear() {
	s.container.Clear()
}
