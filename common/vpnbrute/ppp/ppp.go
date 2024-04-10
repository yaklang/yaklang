package ppp

type PPPAuth interface {
	Auth() bool
	AuthType() string
	AuthCode() uint16
}

type PPPMachine struct {
	Username            string
	Password            string
	AuthType            string
	AuthCode            uint16
	LCPNegotiateIdIndex int
}

// just Negotiate Auth Type
func (m *PPPMachine) GetLCPNegotiateMessageParams() map[string]any {

	mapData := map[string]any{
		"Address":  0xff,
		"Control":  0x03,
		"Protocol": 0xc023,
		"LCP": map[string]any{
			"Code":       1,
			"Identifier": 1,
			"Length":     36,
			//"Options":    ,
		},
	}

	return mapData
}

func (m *PPPMachine) GetAuthOptParam() map[string]any {
	return map[string]any{
		"Code": 3,
	}
}
