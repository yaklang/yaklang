package contextmenu

type PacketActionResult struct {
	Request             []byte
	Response            []byte
	ReplaceRequest      bool
	ReplaceResponse     bool
	RequireConfirmation bool
}

type PacketResultOption func(*PacketActionResult)

func NewPacketResult(options ...PacketResultOption) *PacketActionResult {
	result := &PacketActionResult{}
	for _, option := range options {
		if option != nil {
			option(result)
		}
	}
	return result
}

func WithRequest(request []byte) PacketResultOption {
	return func(result *PacketActionResult) {
		result.Request = append([]byte(nil), request...)
		result.ReplaceRequest = true
	}
}

func WithResponse(response []byte) PacketResultOption {
	return func(result *PacketActionResult) {
		result.Response = append([]byte(nil), response...)
		result.ReplaceResponse = true
	}
}

func WithConfirmation(required bool) PacketResultOption {
	return func(result *PacketActionResult) {
		result.RequireConfirmation = required
	}
}
