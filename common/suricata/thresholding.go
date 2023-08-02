package suricata

type ThresholdingConfig struct {
	ThresholdMode bool
	LimitMode     bool
	Count         int
	Seconds       int
	Track         string
}

func (t *ThresholdingConfig) Repeat() int {
	if t == nil {
		return 1
	}

	if t.Count > 0 {
		return t.Count
	}

	return 1
}
