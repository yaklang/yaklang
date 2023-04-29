package httptpl

type YakPayloads struct {
	raw map[string]*YakPayload
}

type YakPayload struct {
	FromFile string
	Data     []string
}
