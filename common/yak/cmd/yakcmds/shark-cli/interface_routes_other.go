//go:build !darwin

package sharkcli

import "github.com/yaklang/yaklang/common/utils/netutil/routewrapper"

func loadDefaultRoutes() ([]routewrapper.Route, error) {
	router, err := routewrapper.NewRouteWrapper()
	if err != nil {
		return nil, err
	}
	return router.DefaultRoutes()
}
