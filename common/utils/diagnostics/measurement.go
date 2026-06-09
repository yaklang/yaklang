package diagnostics

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// Measurement aggregates stats per lab name. Per-run detail is in Recorder.Steps.
type Measurement struct {
	mu         sync.Mutex
	Name       string
	Total      time.Duration
	Min        time.Duration
	Max        time.Duration
	Count      uint64
	ErrorCount uint64
	Steps      []time.Duration
}

func newMeasurement(name string) *Measurement {
	return &Measurement{Name: name}
}

func (m *Measurement) absorb(duration time.Duration, stepIndex int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Count == 0 {
		m.Min, m.Max = duration, duration
	} else {
		if duration < m.Min {
			m.Min = duration
		}
		if duration > m.Max {
			m.Max = duration
		}
	}
	m.Total += duration
	m.Count++
	if stepIndex >= 0 {
		for len(m.Steps) <= stepIndex {
			m.Steps = append(m.Steps, 0)
		}
		m.Steps[stepIndex] += duration
	}
}

func (m *Measurement) markError() {
	m.mu.Lock()
	m.ErrorCount++
	m.mu.Unlock()
}

func (m *Measurement) snapshot() Measurement {
	m.mu.Lock()
	defer m.mu.Unlock()
	steps := m.Steps
	if len(steps) > 0 {
		steps = append([]time.Duration(nil), steps...)
	}
	return Measurement{
		Name: m.Name, Total: m.Total, Min: m.Min, Max: m.Max,
		Count: m.Count, ErrorCount: m.ErrorCount, Steps: steps,
	}
}

func (m Measurement) Average() time.Duration {
	if m.Count == 0 {
		return 0
	}
	return m.Total / time.Duration(m.Count)
}

func (m Measurement) String() string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("----------- Measurement [%s] --------------------\n", m.Name))
	b.WriteString(fmt.Sprintf("-------- Measurement %s\tCount %v\n", m.Name, m.Count))
	if m.Count == 0 {
		return b.String()
	}
	b.WriteString(fmt.Sprintf("%s--all\tTime: %v\tCount: %v\tAvg: %v\n", m.Name, m.Total, m.Count, m.Average()))
	b.WriteString(fmt.Sprintf("%s--range\tMin: %v\tMax: %v\n", m.Name, m.Min, m.Max))
	if len(m.Steps) > 1 {
		for i, t := range m.Steps {
			if t > 0 {
				b.WriteString(fmt.Sprintf("%s-%-4d\tTime: %v\n", m.Name, i+1, t))
			}
		}
	}
	if m.ErrorCount > 0 {
		b.WriteString(fmt.Sprintf("%s--errors\tCount: %v\n", m.Name, m.ErrorCount))
	}
	return b.String()
}
