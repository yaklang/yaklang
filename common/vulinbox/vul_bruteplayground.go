package vulinbox

import (
	"bytes"
	_ "embed"
	"fmt"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"text/template"
	"time"

	"github.com/yaklang/yaklang/common/filter"
	"github.com/yaklang/yaklang/common/utils/omap"
)

//go:embed html/vul_bruteplayground_orderpage.html
var vulInBrutePlayground string

func (s *VulinServer) registerBrutePlayground() {
	router := s.router

	bruteplayground := router.Name("遍历与爆破练习").Subrouter()

	infos := make(map[int]string)
	for i := 0; i < 1000; i++ {
		// 生成随机的6位数字作为订单号
		orderNum := rand.Intn(10000)
		infos[orderNum] = "13" + strconv.FormatInt(int64(10000000+rand.Intn(89999999)), 10)
	}

	render := func(
		writer http.ResponseWriter,
		orderId string, pathString string,
		lastLevel, nextLevel string,
		errReason string,
	) string {
		tmpl, err := template.New("vul_bruteplayground_orderpage").Parse(string(vulInBrutePlayground))
		if err != nil {
			http.Error(writer, err.Error(), http.StatusInternalServerError)
			return ""
		}

		if errReason != "" {
			orderId = ""
			var buf bytes.Buffer
			tmpl.Execute(&buf, map[string]any{
				"QueryId":   orderId,
				"Path":      pathString,
				"LastLevel": lastLevel,
				"NextLevel": nextLevel,
				"ErrReason": errReason,
			})
			return buf.String()
		}

		// 根据订单ID获取数据
		orderId = strings.TrimSpace(orderId)
		orderIdInt, err := strconv.Atoi(orderId)
		if err != nil {
			var buf bytes.Buffer
			tmpl.Execute(&buf, map[string]any{
				"QueryId":   orderId,
				"Path":      pathString,
				"LastLevel": lastLevel,
				"NextLevel": nextLevel,
				"ErrReason": errReason,
			})
			return buf.String()
		}

		// 获取订单信息
		phone, ok := infos[orderIdInt]
		if !ok {
			var buf bytes.Buffer
			tmpl.Execute(&buf, map[string]any{
				"QueryId":   orderId,
				"ErrReason": "订单号不存在",
				"Path":      pathString,
				"NextLevel": nextLevel,
				"LastLevel": lastLevel,
			})
			return buf.String()
		}

		// 渲染订单详情
		var buf bytes.Buffer
		tmpl.Execute(&buf, map[string]any{
			"OrderId":   orderId,
			"Name":      "客户" + orderId,
			"Phone":     phone,
			"Path":      pathString,
			"NextLevel": nextLevel,
			"LastLevel": lastLevel,
			"ErrReason": errReason,
		})
		return buf.String()
	}

	const (
		baseOrderPath       = "/bruteplayground"
		simpleOrderPath     = baseOrderPath + "/by-order-id"
		dateOrderPath       = baseOrderPath + "/by-order-id-2"
		orderIdNTraceIdPath = baseOrderPath + "/by-order-id-3"
		defaultOrderId      = "3321"
	)

	// 生成今日日期格式的订单号
	todayOrderId := time.Now().Format("20060102") + "0001"

	// 构建查询参数
	simpleQuery := fmt.Sprintf("orderId=%s", defaultOrderId)
	dateQuery := fmt.Sprintf("orderId=%s", todayOrderId)
	orderIdNTraceIdQuery := fmt.Sprintf("orderId=%s&tradeId=%s", defaultOrderId, "1234567890")

	var virtualRoute = omap.NewEmptyOrderedMap[string, map[string]string]()
	virtualRoute.Push(map[string]string{
		"path":  simpleOrderPath,
		"title": "订单详情页面（订单号为4位数字 0-9999）",
		"query": simpleQuery,
	})
	virtualRoute.Push(map[string]string{
		"path":  dateOrderPath,
		"title": "订单详情页面（订单号为今日日期+4位数字 0000-9999）",
		"query": dateQuery,
	})
	virtualRoute.Push(map[string]string{
		"path":  orderIdNTraceIdPath,
		"title": "订单详情页面：traceId不重复（爆破/遍历订单号）",
		"query": orderIdNTraceIdQuery,
	})

	getQuery := func(i int) string {
		r := virtualRoute.GetByIndexMust(i)
		return r["query"]
	}
	getPath := func(i int) string {
		r := virtualRoute.GetByIndexMust(i)
		return r["path"]
	}
	getPathWithQuery := func(i int) string {
		return getPath(i) + "?" + getQuery(i)
	}

	traceIdFilter := filter.NoCacheNewFilter()

	routes := []*VulInfo{
		{
			DefaultQuery: getQuery(0),
			Path:         getPath(0),
			Title:        "订单详情页面（爆破 / 遍历订单号为4位数字 0-9999）",
			Handler: func(writer http.ResponseWriter, request *http.Request) {
				orderId := request.URL.Query().Get("orderId")
				writer.Write([]byte(render(
					writer, orderId,
					getPathWithQuery(0),
					"",
					getPathWithQuery(1),
					"",
				)))
			},
			RiskDetected: false,
		},
		{
			DefaultQuery: dateQuery,
			Path:         dateOrderPath,
			Title:        "订单详情页面（爆破 / 遍历订单号为今日日期+4位数字 0000-9999）",
			Handler: func(writer http.ResponseWriter, request *http.Request) {
				orderId := request.URL.Query().Get("orderId")
				if !strings.HasPrefix(orderId, time.Now().Format("20060102")) {
					writer.Write([]byte(render(
						writer, "",
						getPathWithQuery(1),
						getPathWithQuery(0),
						"",
						"订单号格式错误",
					)))
					return
				}
				orderId = strings.TrimPrefix(orderId, time.Now().Format("20060102"))
				writer.Write([]byte(render(
					writer, orderId,
					getPathWithQuery(1),
					getPathWithQuery(0),
					"",
					"",
				)))
			},
			RiskDetected: false,
		},
		{
			DefaultQuery: orderIdNTraceIdQuery,
			Path:         orderIdNTraceIdPath,
			Title:        "订单详情页面：traceId不重复（爆破/遍历订单号）",
			Handler: func(writer http.ResponseWriter, request *http.Request) {
				qrs := request.URL.Query()
				traceId := qrs.Get("tradeId")
				if traceId == "" {
					writer.Write([]byte(render(
						writer, "",
						getPathWithQuery(2),
						getPathWithQuery(1),
						"",
						"traceId is required",
					)))
					return
				}

				if traceIdFilter.Exist(traceId) {
					writer.Write([]byte(render(
						writer, "",
						getPathWithQuery(2),
						getPathWithQuery(1),
						"",
						"traceId is already used",
					)))
					return
				}
				traceIdFilter.Insert(traceId)
				orderId := qrs.Get("orderId")
				writer.Write([]byte(render(
					writer, orderId,
					getPathWithQuery(2),
					getPathWithQuery(1),
					"",
					"",
				)))
			},
			RiskDetected: false,
		},
	}
	for _, v := range routes {
		addRouteWithVulInfo(bruteplayground, v)
	}
}
