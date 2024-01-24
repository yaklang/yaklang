package vulinbox

import (
	_ "embed"
	"encoding/json"
	"fmt"

	// "fmt"
	"net/http"
	"strconv"
	"text/template"
)

//go:embed html/mall/vul_mall_cart.html
var mallCartPage []byte

func (s *VulinServer) mallCartRoute() {
	// var router = s.router
	// mallcartGroup := router.PathPrefix("/mall").Name("商城").Subrouter()
	mallcartRoutes := []*VulInfo{

		//购物车信息
		{
			DefaultQuery: "",
			Path:         "/user/cart",
			Handler: func(writer http.ResponseWriter, request *http.Request) {
				//验证是否登陆
				_, err := s.database.mallAuthenticate(writer, request)
				if err != nil {
					return
				}
				// var Username = realuser.Role
				type TemplateData struct {
					CartInfo []UserCart
					Username string
				}
				// 通过 id 获取用户购物车信息
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
				cartInfo, err := s.database.GetCart(int(i))

				for i := range cartInfo {
					cartInfo[i].TotalPrice = cartInfo[i].ProductPrice * float64(cartInfo[i].ProductQuantity)
					// fmt.Println(userCart.ProductPrice)
				}
				data := TemplateData{
					Username: userInfo.Username,
					CartInfo: cartInfo,
				}

				if err != nil {
					writer.Write([]byte(err.Error()))
					writer.WriteHeader(500)
					return
				}
				// 打印返回值
				fmt.Printf("cartInfo: %+v\n", cartInfo)

				//如果购物车数量不为0，返回购物车页面信息
				writer.Header().Set("Content-Type", "text/html")
				tmpl, err := template.New("cart").Parse(string(mallCartPage))

				if err != nil {
					writer.WriteHeader(http.StatusInternalServerError)
					writer.Write([]byte("Internal error, cannot render user cartInfo1"))
					return
				}
				err = tmpl.Execute(writer, data)
				if err != nil {
					writer.WriteHeader(http.StatusInternalServerError)
					writer.Write([]byte("Internal error, cannot render user cartInfo2"))
					return
				}

			},
		},

		// 加入购物车
		{
			DefaultQuery: "",
			Path:         "/cart/add",
			Handler: func(writer http.ResponseWriter, request *http.Request) {
				_, err := s.database.mallAuthenticate(writer, request)
				if err != nil {
					return
				}

				// 解析前端传来的JSON数据
				var cart UserCart
				err = json.NewDecoder(request.Body).Decode(&cart)
				if err != nil {
					http.Error(writer, err.Error(), http.StatusBadRequest)
					return
				}

				// 调用函数将商品添加到购物车
				err = s.database.AddCart(cart.UserID, cart)
				if err != nil {
					http.Error(writer, err.Error(), http.StatusInternalServerError)
					return
				}

				writer.WriteHeader(http.StatusOK)
				writer.Write([]byte("加入购物车成功"))

			},
		},
		//获取购物车商品数量
		{
			DefaultQuery: "",
			Path:         "/cart/count",
			Handler: func(writer http.ResponseWriter, request *http.Request) {
				_, err := s.database.mallAuthenticate(writer, request)
				if err != nil {
					return
				}
				//解析前端传来的JSON数据
				var RequestBody struct {
					UserID int `json:"userID"`
				}
				// var body RequestBody
				var ID UserCart

				//获取前端传递的userID
				err = json.NewDecoder(request.Body).Decode(&RequestBody)
				if err != nil {
					writer.WriteHeader(500)
					writer.Write([]byte(err.Error()))
					return
				}
				ID.UserID = RequestBody.UserID
				if err != nil {
					writer.WriteHeader(500)
					writer.Write([]byte("UserID must be a number"))
					return
				}

				//调用函数获取购物车商品数量
				cartSum, err := s.database.GetUserCartCount(ID.UserID)
				if err != nil {
					return
				}
				writer.WriteHeader(http.StatusOK)
				writer.Write([]byte(strconv.Itoa(int(cartSum))))

			},
		},

		// RiskDetected: true,
		//检查购物车是否存在
		{
			DefaultQuery: "",
			Path:         "/cart/check",
			Handler: func(writer http.ResponseWriter, request *http.Request) {
				_, err := s.database.mallAuthenticate(writer, request)
				if err != nil {
					return
				}
				//解析前端传来的JSON数据
				var RequestBody struct {
					UserID      int    `json:"userID"`
					ProductName string `json:"productName"`
				}
				var ID UserCart

				//获取前端传递的userID和productName
				err = json.NewDecoder(request.Body).Decode(&RequestBody)
				if err != nil {
					writer.WriteHeader(500)
					writer.Write([]byte(err.Error()))
					return
				}
				ID.UserID = RequestBody.UserID
				if err != nil {
					writer.WriteHeader(500)
					writer.Write([]byte("UserID must be a number"))
					return
				}
				ID.ProductName = RequestBody.ProductName

				//调用函数检查购物车是否存在该商品
				cartExists, err := s.database.CheckCart(ID.UserID, ID.ProductName)
				if err != nil {
					return
				}
				writer.WriteHeader(http.StatusOK)
				writer.Write([]byte(strconv.FormatBool(cartExists)))

			},
		},

		//购物车商品加一
		{
			DefaultQuery: "",
			Path:         "/cart/addOne",
			Handler: func(writer http.ResponseWriter, request *http.Request) {
				_, err := s.database.mallAuthenticate(writer, request)
				if err != nil {
					return
				}
				//解析前端传来的JSON数据
				var RequestBody struct {
					UserID      string `json:"userID"`
					ProductName string `json:"productName"`
				}
				var ID UserCart

				//获取前端传递的userID和productName
				err = json.NewDecoder(request.Body).Decode(&RequestBody)
				if err != nil {
					writer.WriteHeader(500)
					writer.Write([]byte(err.Error()))
					return
				}
				ID.UserID, err = strconv.Atoi(RequestBody.UserID)
				if err != nil {
					writer.WriteHeader(500)
					writer.Write([]byte("UserID must be a number"))
					return
				}
				ID.ProductName = RequestBody.ProductName

				//调用函数购物车商品加一
				err = s.database.AddCartQuantity(ID.UserID, ID.ProductName)
				if err != nil {
					return
				}
				writer.WriteHeader(http.StatusOK)
				writer.Write([]byte("购物车商品加一成功"))

			},
		},
		//购物车商品减一
		{
			DefaultQuery: "",
			Path:         "/cart/subOne",
			Handler: func(writer http.ResponseWriter, request *http.Request) {
				_, err := s.database.mallAuthenticate(writer, request)
				if err != nil {
					return
				}
				//解析前端传来的JSON数据
				var RequestBody struct {
					UserID      string `json:"userID"`
					ProductName string `json:"productName"`
				}
				var ID UserCart

				//获取前端传递的userID和productName
				err = json.NewDecoder(request.Body).Decode(&RequestBody)
				if err != nil {
					writer.WriteHeader(500)
					writer.Write([]byte(err.Error()))
					return
				}
				ID.UserID, err = strconv.Atoi(RequestBody.UserID)
				if err != nil {
					writer.WriteHeader(500)
					writer.Write([]byte("UserID must be a number"))
					return
				}
				ID.ProductName = RequestBody.ProductName

				//调用函数购物车商品减一
				err = s.database.SubCartQuantity(ID.UserID, ID.ProductName)
				if err != nil {
					return
				}
				writer.WriteHeader(http.StatusOK)
				writer.Write([]byte("购物车商品减一成功"))

			},
		},
		//删除购物车商品
		{

			DefaultQuery: "",
			Path:         "/cart/delete",
			Handler: func(writer http.ResponseWriter, request *http.Request) {
				_, err := s.database.mallAuthenticate(writer, request)
				if err != nil {
					return
				}
				//解析前端传来的JSON数据
				var RequestBody struct {
					UserID      string `json:"userID"`
					ProductName string `json:"productName"`
				}
				var ID UserCart

				//获取前端传递的userID和productName
				err = json.NewDecoder(request.Body).Decode(&RequestBody)
				if err != nil {
					writer.WriteHeader(500)
					writer.Write([]byte(err.Error()))
					return
				}
				ID.UserID, err = strconv.Atoi(RequestBody.UserID)
				if err != nil {
					writer.WriteHeader(500)
					writer.Write([]byte("UserID must be a number"))
					return
				}
				ID.ProductName = RequestBody.ProductName

				//调用函数删除购物车商品
				err = s.database.DeleteCartByName(ID.UserID, ID.ProductName)
				if err != nil {
					return
				}
				writer.WriteHeader(http.StatusOK)
				writer.Write([]byte("删除购物车商品成功"))

			},
		},
		//清空购物车
		{
			DefaultQuery: "",
			Path:         "/cart/clear",
			Handler: func(writer http.ResponseWriter, request *http.Request) {
				_, err := s.database.mallAuthenticate(writer, request)
				if err != nil {
					return
				}
				//解析前端传来的JSON数据
				var RequestBody struct {
					UserID int `json:"userID"`
				}
				var ID UserCart

				//获取前端传递的userID
				err = json.NewDecoder(request.Body).Decode(&RequestBody)
				if err != nil {
					writer.WriteHeader(500)
					writer.Write([]byte(err.Error()))
					return
				}
				ID.UserID = RequestBody.UserID

				//调用函数清空购物车
				err = s.database.ClearCart(ID.UserID)
				if err != nil {
					return
				}
				writer.WriteHeader(http.StatusOK)
				writer.Write([]byte("清空购物车成功"))

			},
		},
	}

	for _, v := range mallcartRoutes {
		addRouteWithVulInfo(MallGroup, v)
	}

}
