package vulinbox

import (
	_ "embed"
	"github.com/gorilla/mux"
	"net/http"
)

//go:embed html/mall/vul_mall_index.html
var mallIndexHtml []byte

var MallGroup *mux.Router

func (s *VulinServer) mallIndexRoute() {
	var router = s.router
	MallGroup = router.PathPrefix("/mall").Name("购物商城").Subrouter()
	orderRoutes := []*VulInfo{
		{
			DefaultQuery: "",
			Path:         "/shop/index",
			Title:        "购物商城场景	",
			Handler: func(writer http.ResponseWriter, request *http.Request) {
				if request.Method == http.MethodGet {
					// 返回登录页面
					writer.Header().Set("Content-Type", "text/html")
					writer.Write(mallIndexHtml)
					return
				}

			},
			RiskDetected: true,
		},
	}
	for _, v := range orderRoutes {
		addRouteWithVulInfo(MallGroup, v)
	}

}
