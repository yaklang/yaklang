package facades

import (
	"context"
	"sync"
)

var facadeServers sync.Map

func GetFacadeServer(token string) *FacadeServer {
	v, ok := facadeServers.Load(token)
	if !ok {
		return nil
	}
	return v.(*FacadeServer)
}
func RegisterFacadeServer(ctx context.Context, token string, server *FacadeServer) {
	facadeServers.Store(token, server)
	go func() {
		<-ctx.Done()
		DeleteFacadeServer(token)
		server.CancelServe()
	}()
}
func DeleteFacadeServer(token string) {
	facadeServers.Delete(token)
}
