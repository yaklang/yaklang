package suricata

type DirectGen struct {
	payload []byte
}

func (c *DirectGen) Gen() []byte {
	return c.payload
}

func parse2DirectGen(rules []*ContentRule) *DirectGen {
	if len(rules) > 0 {
		return &DirectGen{
			payload: rules[0].Content,
		}
	}
	return nil
}
