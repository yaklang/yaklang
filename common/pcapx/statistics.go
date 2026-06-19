package pcapx

import "sync"

type Statistics struct {
	LinkLayerStatistics           map[string]int64
	NetworkLayerStatistics        map[string]int64
	TransportationLayerStatistics map[string]int64
	ICMPStatistics                map[string]int64
}

var statisticsMutex = new(sync.Mutex)

var globalStatistics = NewStatistics()

func GetGlobalStatistics() *Statistics {
	return globalStatistics
}

// GetStatistics 获取 pcapx 注入流量过程中累计的统计信息(链路层、网络层、传输层)
// 在 yak 中通过 pcapx.GetStatistics 调用
// 参数:
//   - 无
//
// 返回值:
//   - 一个统计信息对象，包含各层地址命中计数
//
// Example:
// ```
// // 该示例为示意性用法：读取 pcapx 流量统计
// stat = pcapx.GetStatistics()
// println(stat)
// ```
func getStatistics() (result *Statistics) {
	defer func() {
		result = globalStatistics
	}()
	return
}

func NewStatistics() *Statistics {
	return &Statistics{
		LinkLayerStatistics:           make(map[string]int64),
		NetworkLayerStatistics:        make(map[string]int64),
		TransportationLayerStatistics: make(map[string]int64),
		ICMPStatistics:                make(map[string]int64),
	}
}

func (s *Statistics) AddLinkLayerStatistics(name string) {
	statisticsMutex.Lock()
	defer statisticsMutex.Unlock()
	if s.LinkLayerStatistics == nil {
		s.LinkLayerStatistics = make(map[string]int64)
	}
	s.LinkLayerStatistics[name]++
}

func (s *Statistics) AddNetworkLayerStatistics(name string) {
	statisticsMutex.Lock()
	defer statisticsMutex.Unlock()
	if s.NetworkLayerStatistics == nil {
		s.NetworkLayerStatistics = make(map[string]int64)
	}
	s.NetworkLayerStatistics[name]++
}

func (s *Statistics) AddTransportationLayerStatistics(name string) {
	statisticsMutex.Lock()
	defer statisticsMutex.Unlock()
	if s.TransportationLayerStatistics == nil {
		s.TransportationLayerStatistics = make(map[string]int64)
	}
	s.TransportationLayerStatistics[name]++
}

func (s *Statistics) AddICMPStatistics(name string) {
	statisticsMutex.Lock()
	defer statisticsMutex.Unlock()
	if s.ICMPStatistics == nil {
		s.ICMPStatistics = make(map[string]int64)
	}
	s.ICMPStatistics[name]++
}
