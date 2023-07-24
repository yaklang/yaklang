package vulinbox

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"github.com/gorilla/mux"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/vulinbox/verificationcode"
	"github.com/yaklang/yaklang/common/vulinboxagentproto"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"text/template"
)

//go:embed route.html
var routeHtml []byte

//go:embed auto_route.html
var autoRouteHtml []byte

type GroupedRoutes struct {
	GroupName string
	SafeStyle string
	VulInfos  []*VulInfo
}

type VulInfo struct {
	Handler       func(http.ResponseWriter, *http.Request) `json:"-"`
	Path          string
	DefaultQuery  string
	Title         string // 名称
	Detected      bool   // 是否能检出
	ExpectedValue string // 期望值
	UnSafe        bool
}

func (s *VulinServer) init() {
	if s.wsAgent.wChan == nil {
		s.wsAgent.wChan = make(chan any, 10000)
	}

	router := s.router

	// FE AND FEEDBACK
	fe := http.FileServer(http.FS(staticFS))
	router.NotFoundHandler = http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		/* load to agent feedback */
		reqRaw, err := utils.HttpDumpWithBody(request, true)
		if err != nil {
			log.Errorf("dump request failed: %v", err)
		}
		if len(reqRaw) > 0 {
			s.wsAgent.TrySend(vulinboxagentproto.NewDataBackAction("http-request", string(reqRaw)))
		}

		if strings.HasPrefix(request.URL.Path, "/static") {
			var u, _ = lowhttp.ExtractURLFromHTTPRequest(request, true)
			if u != nil {
				log.Infof("request static file: %v", u.Path)
				// request.URL.Path = strings.TrimLeft(request.URL.Path, "/")
			}
			fe.ServeHTTP(writer, request)
			return
		}
		log.Infof("404 for %s", request.URL.Path)
		http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			writer.WriteHeader(404)
			writer.Write([]byte("404 not found"))
		}).ServeHTTP(writer, request)
	})
	router.Use(func(handler http.Handler) http.Handler {
		return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			reqRaw, err := utils.HttpDumpWithBody(request, false)
			if err != nil {
				log.Errorf("dump request failed: %v", err)
			}
			if len(reqRaw) > 0 {
				s.wsAgent.TrySend(vulinboxagentproto.NewDataBackAction("http-request", string(reqRaw)))
			}
			handler.ServeHTTP(writer, request)
		})
	})
	router.HandleFunc("/", func(writer http.ResponseWriter, request *http.Request) {
		var renderedData string
		if s.safeMode {
			renderedData = `<script>const c = document.getElementById("safestyle"); if (c) c.style.display='none';</script>`
		} else {
			renderedData = ""
		}

		res := strings.Replace(string(autoRouteHtml), "{{.safescript}}", renderedData, 1)

		// Parse and execute template
		t, err := template.New("vulRouter").Parse(res)
		if err != nil {
			panic(err)
		}

		writer.Header().Set("Content-Type", "text/html; charset=UTF8")

		routesData := s.groupedRoutesCache
		err = t.Execute(writer, routesData) // pass the data map to the Execute function instead of routesData directly
		if err != nil {
			http.Error(writer, err.Error(), http.StatusInternalServerError)
		}

	})

	// agent ws connector
	s.registerWSAgent()

	// 通用型
	s.registerSQLinj()
	s.registerXSS()
	s.registerSSRF()
	s.registerMockVulShiro()
	s.registerExprInj()
	s.registerWebsocket()
	s.registerLoginRoute()
	s.registerCryptoJS()
	s.registerCryptoSM()
	s.registerUploadCases()

	// 业务型
	s.registerUserRoute()

	// 验证码
	verificationcode.Register(router)

	s.registerJSONP()
	s.registerPostMessageIframeCase()
	s.registerSensitive()

	// 靶场是否是安全的？
	if !s.safeMode {
		s.registerPingCMDI()
	}

	s.genRoute()

	s.registerVulRouter()

}

var once sync.Once

func (s *VulinServer) genRoute() {
	var routesData []*GroupedRoutes
	var err error
	once.Do(func() {
		groups := make(map[string][]*VulInfo)
		err = s.router.Walk(func(route *mux.Route, router *mux.Router, ancestors []*mux.Route) error {
			// 分组名
			var groupName string
			// 如果当前路由有父路由，就代表这个路由在一个 group 中
			if len(ancestors) > 0 {
				groupName = ancestors[0].GetName()
			}

			if groupName != "" {
				info := route.GetName()

				var vulnInfo *VulInfo
				err = json.Unmarshal([]byte(info), &vulnInfo)
				if err != nil {
					log.Warnf("unmarshal route info failed: %v", err)
					return nil
				}
				// 一个组中不是所有的路由都有名称，只有需要在前端展示的路由才有名称
				if vulnInfo.Title != "" {
					realRoutePath, err := route.GetPathTemplate()
					if err != nil {
						log.Errorf("get route name failed: %v", err)
						return nil
					}
					if strings.HasSuffix(realRoutePath, vulnInfo.Path) {
						vulnInfo.Path = realRoutePath
					}
					if vulnInfo.DefaultQuery != "" {
						params, err := url.ParseQuery(vulnInfo.DefaultQuery)
						if err != nil {
							// handle error
						}

						for key, values := range params {
							for i, value := range values {
								params[key][i] = url.QueryEscape(value)
							}
						}
						vulnInfo.DefaultQuery = "?" + params.Encode()
					}
					if vulnInfo.UnSafe {
						groupName += " (Unsafe Mode)"
					}
					groups[groupName] = append(groups[groupName], vulnInfo)
				}
			}
			return nil
		})
		if err != nil {
			return
		}

		for groupName, routes := range groups {
			id := ""
			if strings.Contains(groupName, "Unsafe Mode") {
				// 当一个变量被作为 HTML attribute 插入时，无法使用 html/template
				id = `id="safestyle"`
			}
			routesData = append(routesData, &GroupedRoutes{
				GroupName: groupName,
				VulInfos:  routes,
				SafeStyle: id,
			})
		}
		s.groupedRoutesCache = routesData
	})
	if err != nil {
		//http.Error(writer, err.Error(), http.StatusInternalServerError)
		return
	}
}

func (s *VulinServer) registerVulRouter() {
	s.router.HandleFunc("/vul/route", func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		err := json.NewEncoder(writer).Encode(s.groupedRoutesCache)
		if err != nil {
			http.Error(writer, fmt.Sprintf("Error encoding groupedRoutesCache: %v", err), http.StatusInternalServerError)
		}

	})

}

func addRouteWithVulInfo(router *mux.Router, info *VulInfo) {
	infoStr, err := json.Marshal(info)
	if err != nil {
		log.Errorf("marshal vuln info failed: %v", err)
		return
	}
	router.HandleFunc(info.Path, info.Handler).Name(string(infoStr))
}

func (s *VulinServer) GetGroupVulInfo(group string) []*VulInfo {
	//	遍历 s.groupedRoutesCache 获取 s.groupedRoutesCache 中的指定的 []*vulInfo
	for _, groupInfo := range s.groupedRoutesCache {
		if groupInfo.GroupName == group {
			return groupInfo.VulInfos
		}
	}
	return nil
}
