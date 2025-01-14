package vulinbox

import (
	"bytes"
	_ "embed"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"text/template"
)

//go:embed html/vul_bruteplayground_orderpage.html
var vulInBrutePlayground string

func (s *VulinServer) registerBrutePlayground() {
	router := s.router

	bruteplayground := router.Name("遍历与爆破练习").Subrouter()

	infos := make(map[int]string)
	for i := 0; i < 1000; i++ {
		// 生成随机的6位数字作为订单号
		orderNum := rand.Intn(1000000)
		infos[orderNum] = "13" + strconv.FormatInt(int64(10000000+rand.Intn(89999999)), 10)
	}

	render := func(writer http.ResponseWriter, orderId string) string {
		tmpl, err := template.New("vul_bruteplayground_orderpage").Parse(string(vulInBrutePlayground))
		if err != nil {
			http.Error(writer, err.Error(), http.StatusInternalServerError)
			return ""
		}

		// 根据订单ID获取数据
		orderId = strings.TrimSpace(orderId)
		orderIdInt, err := strconv.Atoi(orderId)
		if err != nil {
			var buf bytes.Buffer
			tmpl.Execute(&buf, map[string]any{
				"QueryId": orderId,
			})
			return buf.String()
		}

		// 获取订单信息
		phone, ok := infos[orderIdInt]
		if !ok {
			var buf bytes.Buffer
			tmpl.Execute(&buf, map[string]any{
				"QueryId": orderId,
			})
			return buf.String()
		}

		// 渲染订单详情
		var buf bytes.Buffer
		tmpl.Execute(&buf, map[string]any{
			"OrderId": orderId,
			"Name":    "客户" + orderId,
			"Phone":   phone,
		})
		return buf.String()
	}

	routes := []*VulInfo{
		{
			DefaultQuery: "orderId=123456",
			Path:         "/bruteplayground/by-order-id",
			Title:        "订单详情页面",
			Handler: func(writer http.ResponseWriter, request *http.Request) {
				orderId := request.URL.Query().Get("orderId")
				writer.Write([]byte(render(writer, orderId)))
			},
			RiskDetected: false,
		},
	}
	for _, v := range routes {
		addRouteWithVulInfo(bruteplayground, v)
	}
}
