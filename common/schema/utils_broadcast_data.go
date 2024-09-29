package schema

type BroadCastHandler struct {
	handler func(typeString string, data any)
}

func (b *BroadCastHandler) Call(typeString string, data any) {
	if b == nil || b.handler == nil {
		return
	}
	b.handler(typeString, data)
}

var broadcastData *BroadCastHandler = new(BroadCastHandler)

func SetBroadCast_Data(f func(typeString string, data any)) {
	broadcastData.handler = f
}

func GetBroadCast_Data() *BroadCastHandler {
	return broadcastData
}
