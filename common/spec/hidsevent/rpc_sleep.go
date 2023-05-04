package hidsevent

type RpcSleepRequest struct {
	Seconds int `json:"seconds"`
}

type RpcSleepResponse struct {
	Ok bool `json:"ok"`
}
