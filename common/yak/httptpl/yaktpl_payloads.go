package httptpl

type YakPayloads struct {
	raw map[string]*YakPayload
}

type YakPayload struct {
	FromFile string
	Data     []string
}

func (y *YakPayloads) GetRawPayloads() map[string]*YakPayload {
	return y.raw
}