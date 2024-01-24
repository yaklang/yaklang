package vulinbox

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"text/template"
)

//go:embed html/mall/vul_mall_order.html
var mallOrderPage []byte

func (s *VulinServer) mallOrderRoute() {
	// var router = s.router
	// mallcartGroup := router.PathPrefix("/mall").Name("商城").Subrouter()
	mallorderRoutes := []*VulInfo{
		//查询订单
		{
			DefaultQuery: "",
			Path:         "/user/order",
			Handler: func(writer http.ResponseWriter, request *http.Request) {
				//验证是否登陆
				_, err := s.database.mallAuthenticate(writer, request)
				if err != nil {
					return
				}
				// var Username = realuser.Role
				type TemplateOrder struct {
					OrderInfo []UserOrder
					Username  string
				}
				// 通过 id 获取用户订单信息
				var a = request.URL.Query().Get("id")
				i, err := strconv.ParseInt(a, 10, 64)
				if err != nil {
					writer.Write([]byte(err.Error()))
					writer.WriteHeader(500)
					return
				}
				userInfo, err := s.database.GetUserById(int(i))
				if err != nil {
					writer.Write([]byte(err.Error()))
					writer.WriteHeader(500)
					return
				}
				orderInfo, err := s.database.QueryOrder(int(i))
				if err != nil {
					writer.Write([]byte(err.Error()))
					writer.WriteHeader(500)
					return
				}
				// 打印返回值
				fmt.Printf("orderInfo: %+v\n", orderInfo)
				data := TemplateOrder{
					Username:  userInfo.Username,
					OrderInfo: orderInfo,
				}
				t, err := template.New("mallOrder").Parse(string(mallOrderPage))
				if err != nil {
					writer.Write([]byte(err.Error()))
					writer.WriteHeader(500)
					return
				}
				err = t.Execute(writer, data)
				if err != nil {
					writer.Write([]byte(err.Error()))
					writer.WriteHeader(500)
					return
				}
			},
		},
		// 提交订单
		{
			DefaultQuery: "",
			Path:         "/order/submit",
			Handler: func(writer http.ResponseWriter, request *http.Request) {
				_, err := s.database.mallAuthenticate(writer, request)
				if err != nil {
					return
				}
				type OrderItem struct {
					ProductName string //商品名称
					Quantity    string //数量
					TotalPrice  string //总价
				}
				type FirstUserOrder struct {
					UserID         int         //用户ID
					OrderItems     []OrderItem //订单项
					DeliveryStatus string      //发货状态
				}
				// 解析前端传来的JSON数据
				var order UserOrder
				var FirstOrder FirstUserOrder
				err = json.NewDecoder(request.Body).Decode(&FirstOrder)
				if err != nil {
					http.Error(writer, err.Error(), http.StatusBadRequest)
					return
				}
				//终端打印order
				fmt.Printf("FirstOrder: %+v\n", FirstOrder)

				// 调用函数提交订单
				for _, item := range FirstOrder.OrderItems {
					order.ProductName = item.ProductName
					order.Quantity, err = strconv.Atoi(item.Quantity)
					order.TotalPrice, err = strconv.ParseFloat(item.TotalPrice, 64)
					order.DeliveryStatus = "已发货"
					err = s.database.AddOrder(FirstOrder.UserID, order)
					if err != nil {
						http.Error(writer, err.Error(), http.StatusInternalServerError)
						return
					}
				}

				writer.WriteHeader(http.StatusOK)
				writer.Write([]byte("订单提交成功"))
			},
		},
		//删除订单
		{
			DefaultQuery: "",
			Path:         "/order/delete",
			Handler: func(writer http.ResponseWriter, request *http.Request) {
				_, err := s.database.mallAuthenticate(writer, request)
				if err != nil {
					return
				}

				// 解析前端传来的JSON数据
				var order UserOrder
				err = json.NewDecoder(request.Body).Decode(&order)
				if err != nil {
					http.Error(writer, err.Error(), http.StatusBadRequest)
					return
				}

				// 调用函数删除订单
				err = s.database.DeleteOrder(order.UserID, order.ProductName)
				if err != nil {
					http.Error(writer, err.Error(), http.StatusInternalServerError)
					return
				}

				writer.WriteHeader(http.StatusOK)
				writer.Write([]byte("订单退货成功"))
			},
		},
	}
	for _, v := range mallorderRoutes {
		addRouteWithVulInfo(MallGroup, v)
	}
}
