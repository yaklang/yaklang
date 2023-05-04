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
