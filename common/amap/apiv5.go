package amap

import (
	"context"
	"fmt"
	"net/url"
	"strings"
)

// V5响应基础结构
type V5BaseResponse struct {
	Status   string `json:"status"`
	Info     string `json:"info"`
	InfoCode string `json:"infocode"`
}

// IsSuccess returns true if the API call was successful.
func (r V5BaseResponse) IsSuccess() bool {
	return r.Status == "1"
}

// Error implements the error interface for API response errors.
func (r V5BaseResponse) Error() string {
	return "amap API error: " + r.Info
}

// V5路径规划策略常量
const (
	// 驾车路径规划策略
	V5DrivingStrategyFastest       = 0 // 速度优先
	V5DrivingStrategyFuel          = 1 // 避免拥堵
	V5DrivingStrategyDistance      = 2 // 距离优先
	V5DrivingStrategyFree          = 3 // 不走收费路段
	V5DrivingStrategyMultiStrategy = 4 // 多策略（同时返回速度优先、费用优先、距离优先的结果）
	V5DrivingStrategyFeeTime       = 5 // 费用优先
	V5DrivingStrategyFeeDistance   = 6 // 躲避拥堵+费用优先
	V5DrivingStrategyTimeDistance  = 7 // 躲避拥堵+距离优先
	V5DrivingStrategyNotHighway    = 8 // 躲避高速
	V5DrivingStrategyOnlyHighway   = 9 // 高速优先
)

// 驾车路径规划请求参数
type V5DrivingRequest struct {
	Origin         Location   // 必填，起点经纬度
	Destination    Location   // 必填，终点经纬度
	Strategy       int        // 可选，路径规划策略，默认为速度优先
	Waypoints      []Location // 可选，途经点
	ShowFields     []string   // 可选，返回结果控制，cost/navi/polyline
	AvoidRoad      []string   // 可选，避让道路名
	AvoidPoly      string     // 可选，避让区域
	ProvinceCross  bool       // 可选，是否可以跨省
	RoadNetworkOpt int        // 可选，道路网络类型，0-所有道路（默认），1-高速及以上，2-国道及以上，3-省道及以上，4-县道及以上
	CarType        int        // 可选，车辆类型，1-小车（默认），2-货车
	PlateNumber    string     // 可选，车牌号，用于规避限行
	PlateProvince  string     // 可选，车牌省份，用于规避限行
	FerryType      int        // 可选，使用轮渡，0-不使用（默认），1-使用
	// 补充货车参数
	Height      float64 // 可选，货车高度，单位：米，取值[0,10]，默认1.6米
	Width       float64 // 可选，货车宽度，单位：米，取值[0,10]，默认2.5米
	Length      float64 // 可选，货车长度，单位：米，取值[0,25]，默认10米
	Weight      float64 // 可选，货车重量，单位：吨，取值[0,100]，默认10吨
	Load        float64 // 可选，货车核定载重，单位：吨，取值[0,100]，默认10吨
	AxisNum     int     // 可选，货车轴数，单位：个，取值[0,50]，默认2轴
	HazardType  string  // 可选，货车危险物类型，取值：0（非危险物）；10001（爆炸品A）...
	ExpandPath  int     // 可选，扩展路径，取值0或1，将多条路径备选方案返回综合起来
	PriceExpand int     // 可选，返回备选收费详情，取值0或1
	Extensions  string  // 可选，返回结果控制，base(默认)或all
}

// 驾车路径规划响应结构
type V5DrivingResponse struct {
	V5BaseResponse
	Route V5DrivingRouteResult `json:"route"`
}

// 驾车路径规划结果
type V5DrivingRouteResult struct {
	Origin      string          `json:"origin"`      // 起点坐标
	Destination string          `json:"destination"` // 终点坐标
	TaxiCost    string          `json:"taxi_cost"`   // 出租车费用，单位：元
	Paths       []V5DrivingPath `json:"paths"`       // 驾车方案列表
}

// 驾车路径规划方案
type V5DrivingPath struct {
	Distance      string          `json:"distance"`       // 方案距离，单位：米
	Duration      string          `json:"duration"`       // 预计行驶时间，单位：秒
	Strategy      string          `json:"strategy"`       // 导航策略
	Tolls         string          `json:"tolls"`          // 此方案收费，单位：元
	TollDistance  string          `json:"toll_distance"`  // 收费路段长度，单位：米
	TrafficLights string          `json:"traffic_lights"` // 红绿灯个数
	Steps         []V5DrivingStep `json:"steps"`          // 导航路段列表
	Restriction   string          `json:"restriction"`    // 限行结果
	Cost          *V5Cost         `json:"cost,omitempty"` // 方案花费
}

// 花费信息
type V5Cost struct {
	Duration   string `json:"duration"`    // 线路耗时
	TaxiCost   string `json:"taxi_cost"`   // 预估出租车费用
	TransitFee string `json:"transit_fee"` // 公交花费
}

// 驾车路径规划导航步骤
type V5DrivingStep struct {
	Instruction     string `json:"instruction"`                // 行走指示
	Road            string `json:"road"`                       // 道路名称
	Distance        string `json:"distance"`                   // 此路段距离，单位：米
	Duration        string `json:"duration"`                   // 此路段预计耗时，单位：秒
	Polyline        string `json:"polyline"`                   // 此路段坐标点串
	Action          string `json:"action,omitempty"`           // 导航主要动作指令
	AssistantAction string `json:"assistant_action,omitempty"` // 导航辅助动作指令
	TollRoad        string `json:"toll_road"`                  // 收费道路
	TollCost        string `json:"toll_cost"`                  // 收费金额
}

// 驾车路径规划
func (c *Client) V5Driving(ctx context.Context, request V5DrivingRequest) (*V5DrivingResponse, error) {
	params := url.Values{}
	params.Set("origin", request.Origin.String())
	params.Set("destination", request.Destination.String())

	if request.Strategy != 0 {
		params.Set("strategy", fmt.Sprintf("%d", request.Strategy))
	}

	if len(request.Waypoints) > 0 {
		waypoints := make([]string, len(request.Waypoints))
		for i, wp := range request.Waypoints {
			waypoints[i] = wp.String()
		}
		params.Set("waypoints", strings.Join(waypoints, ";"))
	}

	if len(request.ShowFields) > 0 {
		params.Set("show_fields", strings.Join(request.ShowFields, ","))
	}

	if len(request.AvoidRoad) > 0 {
		params.Set("avoid_road", strings.Join(request.AvoidRoad, ","))
	}

	if request.AvoidPoly != "" {
		params.Set("avoid_polygons", request.AvoidPoly)
	}

	if request.ProvinceCross {
		params.Set("province_cross", "1")
	}

	if request.RoadNetworkOpt > 0 {
		params.Set("road_network_opt", fmt.Sprintf("%d", request.RoadNetworkOpt))
	}

	if request.CarType > 0 {
		params.Set("car_type", fmt.Sprintf("%d", request.CarType))
	}

	if request.PlateNumber != "" {
		params.Set("plate_number", request.PlateNumber)
	}

	if request.PlateProvince != "" {
		params.Set("plate_province", request.PlateProvince)
	}

	if request.FerryType > 0 {
		params.Set("ferry_type", fmt.Sprintf("%d", request.FerryType))
	}

	// 货车参数
	if request.Height > 0 {
		params.Set("height", fmt.Sprintf("%.1f", request.Height))
	}

	if request.Width > 0 {
		params.Set("width", fmt.Sprintf("%.1f", request.Width))
	}

	if request.Length > 0 {
		params.Set("length", fmt.Sprintf("%.1f", request.Length))
	}

	if request.Weight > 0 {
		params.Set("weight", fmt.Sprintf("%.1f", request.Weight))
	}

	if request.Load > 0 {
		params.Set("load", fmt.Sprintf("%.1f", request.Load))
	}

	if request.AxisNum > 0 {
		params.Set("axis_num", fmt.Sprintf("%d", request.AxisNum))
	}

	if request.HazardType != "" {
		params.Set("hazard_type", request.HazardType)
	}

	if request.ExpandPath > 0 {
		params.Set("expand_path", fmt.Sprintf("%d", request.ExpandPath))
	}

	if request.PriceExpand > 0 {
		params.Set("price_expand", fmt.Sprintf("%d", request.PriceExpand))
	}

	if request.Extensions != "" {
		params.Set("extensions", request.Extensions)
	}

	response := &V5DrivingResponse{}
	err := c.doRequest(ctx, "/v5/direction/driving", params, response)
	return response, err
}

// 步行路径规划请求参数
type V5WalkingRequest struct {
	Origin      Location // 必填，起点经纬度
	Destination Location // 必填，终点经纬度
	ShowFields  []string // 可选，返回结果控制，cost/navi/polyline
	// 补充参数
	AlternativeRoute int    // 可选，返回可选路径，当值为1时，返回一条可选路径
	Extensions       string // 可选，返回结果控制，base(默认)或all
}

// 步行路径规划响应结构
type V5WalkingResponse struct {
	V5BaseResponse
	Route V5WalkingRouteResult `json:"route"`
}

// 步行路径规划结果
type V5WalkingRouteResult struct {
	Origin      string          `json:"origin"`      // 起点坐标
	Destination string          `json:"destination"` // 终点坐标
	Paths       []V5WalkingPath `json:"paths"`       // 步行方案列表
}

// 步行路径规划方案
type V5WalkingPath struct {
	Distance string          `json:"distance"`       // 方案距离，单位：米
	Steps    []V5WalkingStep `json:"steps"`          // 导航路段列表
	Cost     *V5Cost         `json:"cost,omitempty"` // 方案花费
}

// 步行路径规划导航步骤
type V5WalkingStep struct {
	Instruction  string `json:"instruction"`   // 行走指示
	Orientation  string `json:"orientation"`   // 方向
	RoadName     string `json:"road_name"`     // 道路名称
	StepDistance string `json:"step_distance"` // 此路段距离，单位：米
}

// 步行路径规划
func (c *Client) V5Walking(ctx context.Context, request V5WalkingRequest) (*V5WalkingResponse, error) {
	params := url.Values{}
	params.Set("origin", request.Origin.String())
	params.Set("destination", request.Destination.String())

	if len(request.ShowFields) > 0 {
		params.Set("show_fields", strings.Join(request.ShowFields, ","))
	}

	if request.AlternativeRoute > 0 {
		params.Set("alternative_route", fmt.Sprintf("%d", request.AlternativeRoute))
	}

	if request.Extensions != "" {
		params.Set("extensions", request.Extensions)
	}

	response := &V5WalkingResponse{}
	err := c.doRequest(ctx, "/v5/direction/walking", params, response)
	return response, err
}

// 骑行路径规划请求参数
type V5BicyclingRequest struct {
	Origin      Location // 必填，起点经纬度
	Destination Location // 必填，终点经纬度
	ShowFields  []string // 可选，返回结果控制，cost/navi/polyline
	BikeType    int      // 可选，自行车类型，1-普通自行车，2-电动自行车
	// 补充参数
	Extensions string // 可选，返回结果控制，base(默认)或all
}

// 骑行路径规划响应结构
type V5BicyclingResponse struct {
	V5BaseResponse
	Route V5BicyclingRouteResult `json:"route"`
}

// 骑行路径规划结果
type V5BicyclingRouteResult struct {
	Origin      string            `json:"origin"`      // 起点坐标
	Destination string            `json:"destination"` // 终点坐标
	Paths       []V5BicyclingPath `json:"paths"`       // 骑行方案列表
}

// 骑行路径规划方案
type V5BicyclingPath struct {
	Distance string            `json:"distance"`       // 方案距离，单位：米
	Duration string            `json:"duration"`       // 预计骑行时间，单位：秒
	Steps    []V5BicyclingStep `json:"steps"`          // 导航路段列表
	Cost     *V5Cost           `json:"cost,omitempty"` // 方案花费
}

// 骑行路径规划导航步骤
type V5BicyclingStep struct {
	Instruction     string `json:"instruction"`                // 行走指示
	Road            string `json:"road"`                       // 道路名称
	Distance        string `json:"distance"`                   // 此路段距离，单位：米
	Duration        string `json:"duration"`                   // 此路段预计耗时，单位：秒
	Polyline        string `json:"polyline"`                   // 此路段坐标点串
	Action          string `json:"action,omitempty"`           // 导航主要动作指令
	AssistantAction string `json:"assistant_action,omitempty"` // 导航辅助动作指令
}

// 骑行路径规划
func (c *Client) V5Bicycling(ctx context.Context, request V5BicyclingRequest) (*V5BicyclingResponse, error) {
	params := url.Values{}
	params.Set("origin", request.Origin.String())
	params.Set("destination", request.Destination.String())

	if len(request.ShowFields) > 0 {
		params.Set("show_fields", strings.Join(request.ShowFields, ","))
	}

	if request.BikeType > 0 {
		params.Set("bike_type", fmt.Sprintf("%d", request.BikeType))
	}

	if request.Extensions != "" {
		params.Set("extensions", request.Extensions)
	}

	response := &V5BicyclingResponse{}
	err := c.doRequest(ctx, "/v5/direction/bicycling", params, response)
	return response, err
}

// 电动车路径规划请求参数
type V5EBikeRequest struct {
	Origin      Location // 必填，起点经纬度
	Destination Location // 必填，终点经纬度
	ShowFields  []string // 可选，返回结果控制，cost/navi/polyline
	// 补充参数
	Extensions string // 可选，返回结果控制，base(默认)或all
}

// 电动车路径规划响应结构
type V5EBikeResponse struct {
	V5BaseResponse
	Route V5EBikeRouteResult `json:"route"`
}

// 电动车路径规划结果
type V5EBikeRouteResult struct {
	Origin      string        `json:"origin"`      // 起点坐标
	Destination string        `json:"destination"` // 终点坐标
	Paths       []V5EBikePath `json:"paths"`       // 电动车方案列表
}

// 电动车路径规划方案
type V5EBikePath struct {
	Distance string        `json:"distance"`       // 方案距离，单位：米
	Duration string        `json:"duration"`       // 预计骑行时间，单位：秒
	Steps    []V5EBikeStep `json:"steps"`          // 导航路段列表
	Cost     *V5Cost       `json:"cost,omitempty"` // 方案花费
}

// 电动车路径规划导航步骤
type V5EBikeStep struct {
	Instruction     string `json:"instruction"`                // 行走指示
	Road            string `json:"road"`                       // 道路名称
	Distance        string `json:"distance"`                   // 此路段距离，单位：米
	Duration        string `json:"duration"`                   // 此路段预计耗时，单位：秒
	Polyline        string `json:"polyline"`                   // 此路段坐标点串
	Action          string `json:"action,omitempty"`           // 导航主要动作指令
	AssistantAction string `json:"assistant_action,omitempty"` // 导航辅助动作指令
}

// 电动车路径规划
func (c *Client) V5EBike(ctx context.Context, request V5EBikeRequest) (*V5EBikeResponse, error) {
	params := url.Values{}
	params.Set("origin", request.Origin.String())
	params.Set("destination", request.Destination.String())

	if len(request.ShowFields) > 0 {
		params.Set("show_fields", strings.Join(request.ShowFields, ","))
	}

	if request.Extensions != "" {
		params.Set("extensions", request.Extensions)
	}

	response := &V5EBikeResponse{}
	err := c.doRequest(ctx, "/v5/direction/ebike", params, response)
	return response, err
}

// 公交路径规划策略常量
const (
	V5TransitStrategyTime        = 0 // 最快捷模式
	V5TransitStrategyNoSubway    = 1 // 不乘地铁
	V5TransitStrategyMinTransfer = 2 // 最少换乘
	V5TransitStrategyMinWalk     = 3 // 最少步行
	V5TransitStrategyComfortable = 4 // 舒适模式
	V5TransitStrategyNoElevator  = 5 // 不走电梯
	V5TransitStrategyMultiMode   = 6 // 地铁优先
)

// 公交路径规划请求参数
type V5TransitRequest struct {
	Origin      Location // 必填，起点经纬度
	Destination Location // 必填，终点经纬度
	City        string   // 必填，城市名/城市编码
	CityD       string   // 可选，目的地城市，跨城时必填
	Strategy    int      // 可选，策略，默认为最快捷模式
	ShowFields  []string // 可选，返回结果控制，cost/navi/polyline
	NightFlag   int      // 可选，是否考虑夜班车，取值为0或1
	DateType    int      // 可选，出发日期类型，0-今天，1-明天
	Date        string   // 可选，出发日期，yyyy-MM-dd格式，默认为当天
	Time        string   // 可选，出发时间，HH:mm格式，默认为当前时间
	MaxTrans    int      // 可选，最大换乘次数，默认为10
	// 补充参数
	AltCount   int    // 可选，返回可选方案条数，默认3条，取值范围[1,5]
	Extensions string // 可选，返回结果控制，base(默认)或all
}

// 公交路径规划响应结构
type V5TransitResponse struct {
	V5BaseResponse
	Route V5TransitRouteResult `json:"route"`
}

// 公交路径规划结果
type V5TransitRouteResult struct {
	Origin      string          `json:"origin"`      // 起点坐标
	Destination string          `json:"destination"` // 终点坐标
	TaxiCost    string          `json:"taxi_cost"`   // 出租车费用
	Transits    []V5TransitPath `json:"transits"`    // 公交换乘方案
}

// 公交路径规划方案
type V5TransitPath struct {
	Distance        string             `json:"distance"`         // 方案距离，单位：米
	Duration        string             `json:"duration"`         // 预计耗时，单位：秒
	WalkingDistance string             `json:"walking_distance"` // 步行距离，单位：米
	Cost            *V5Cost            `json:"cost,omitempty"`   // 方案花费
	Segments        []V5TransitSegment `json:"segments"`         // 换乘路段
}

// 公交换乘路段
type V5TransitSegment struct {
	Walking  *V5TransitWalking  `json:"walking,omitempty"`  // 步行子路段
	Bus      *V5TransitBus      `json:"bus,omitempty"`      // 公交子路段
	Railway  *V5TransitRailway  `json:"railway,omitempty"`  // 地铁子路段
	Entrance *V5TransitEntrance `json:"entrance,omitempty"` // 地铁入口
	Exit     *V5TransitExit     `json:"exit,omitempty"`     // 地铁出口
}

// 步行子路段
type V5TransitWalking struct {
	Origin      string          `json:"origin"`      // 起点
	Destination string          `json:"destination"` // 终点
	Distance    string          `json:"distance"`    // 距离
	Duration    string          `json:"duration"`    // 耗时
	Steps       []V5WalkingStep `json:"steps"`       // 步行路段
}

// 公交子路段
type V5TransitBus struct {
	BusLines []V5BusLine `json:"buslines"` // 公交线路
}

// 地铁子路段
type V5TransitRailway struct {
	RailLines []V5RailLine `json:"railway"` // 地铁线路
}

// 公交线路
type V5BusLine struct {
	Name      string   `json:"name"`           // 线路名称
	ID        string   `json:"id"`             // 线路ID
	Type      string   `json:"type"`           // 类型
	Distance  string   `json:"distance"`       // 距离
	Duration  string   `json:"duration"`       // 耗时
	Polyline  string   `json:"polyline"`       // 坐标点串
	Departure string   `json:"departure_stop"` // 上车站
	Arrival   string   `json:"arrival_stop"`   // 下车站
	ViaStops  []V5Stop `json:"via_stops"`      // 途径站点
}

// 地铁线路
type V5RailLine struct {
	Name      string   `json:"name"`           // 线路名称
	ID        string   `json:"id"`             // 线路ID
	Type      string   `json:"type"`           // 类型
	Distance  string   `json:"distance"`       // 距离
	Duration  string   `json:"duration"`       // 耗时
	Polyline  string   `json:"polyline"`       // 坐标点串
	Departure string   `json:"departure_stop"` // 上车站
	Arrival   string   `json:"arrival_stop"`   // 下车站
	ViaStops  []V5Stop `json:"via_stops"`      // 途径站点
}

// 站点
type V5Stop struct {
	Name     string `json:"name"`     // 站点名称
	ID       string `json:"id"`       // 站点ID
	Location string `json:"location"` // 站点位置
}

// 地铁入口
type V5TransitEntrance struct {
	Name     string `json:"name"`     // 入口名称
	Location string `json:"location"` // 入口位置
}

// 地铁出口
type V5TransitExit struct {
	Name     string `json:"name"`     // 出口名称
	Location string `json:"location"` // 出口位置
}

// 公交路径规划
func (c *Client) V5Transit(ctx context.Context, request V5TransitRequest) (*V5TransitResponse, error) {
	params := url.Values{}
	params.Set("origin", request.Origin.String())
	params.Set("destination", request.Destination.String())

	// 城市是必填参数
	if request.City == "" {
		return nil, fmt.Errorf("city is required for transit directions")
	}
	params.Set("city", request.City)

	if request.CityD != "" {
		params.Set("cityd", request.CityD)
	}

	if request.Strategy > 0 {
		params.Set("strategy", fmt.Sprintf("%d", request.Strategy))
	}

	if len(request.ShowFields) > 0 {
		params.Set("show_fields", strings.Join(request.ShowFields, ","))
	}

	if request.NightFlag > 0 {
		params.Set("nightflag", fmt.Sprintf("%d", request.NightFlag))
	}

	if request.DateType > 0 {
		params.Set("date_type", fmt.Sprintf("%d", request.DateType))
	}

	if request.Date != "" {
		params.Set("date", request.Date)
	}

	if request.Time != "" {
		params.Set("time", request.Time)
	}

	if request.MaxTrans > 0 {
		params.Set("max_trans", fmt.Sprintf("%d", request.MaxTrans))
	}

	if request.AltCount > 0 {
		params.Set("alt_count", fmt.Sprintf("%d", request.AltCount))
	}

	if request.Extensions != "" {
		params.Set("extensions", request.Extensions)
	}

	response := &V5TransitResponse{}
	err := c.doRequest(ctx, "/v5/direction/transit/integrated", params, response)
	return response, err
}

// 货车路径规划请求参数
type V5TruckRequest struct {
	Origin         Location   // 必填，起点经纬度
	Destination    Location   // 必填，终点经纬度
	Strategy       int        // 可选，路径规划策略，默认为速度优先
	Waypoints      []Location // 可选，途经点
	ShowFields     []string   // 可选，返回结果控制，cost/navi/polyline
	AvoidRoad      []string   // 可选，避让道路名
	AvoidPoly      string     // 可选，避让区域
	ProvinceCross  bool       // 可选，是否可以跨省
	RoadNetworkOpt int        // 可选，道路网络类型，0-所有道路（默认），1-高速及以上，2-国道及以上，3-省道及以上，4-县道及以上
	Height         float64    // 可选，货车高度，单位：米，取值[0,10]，默认1.6米
	Width          float64    // 可选，货车宽度，单位：米，取值[0,10]，默认2.5米
	Length         float64    // 可选，货车长度，单位：米，取值[0,25]，默认10米
	Weight         float64    // 可选，货车重量，单位：吨，取值[0,100]，默认10吨
	Load           float64    // 可选，货车核定载重，单位：吨，取值[0,100]，默认10吨
	AxisNum        int        // 可选，货车轴数，单位：个，取值[0,50]，默认2轴
	HazardType     string     // 可选，货车危险物类型，取值：0（非危险物）；10001（爆炸品A）...
	PlateNumber    string     // 可选，车牌号，用于规避限行
	PlateProvince  string     // 可选，车牌省份，用于规避限行
	FerryType      int        // 可选，使用轮渡，0-不使用（默认），1-使用
	ExpandPath     int        // 可选，扩展路径，取值0或1，将多条路径备选方案返回综合起来
	PriceExpand    int        // 可选，返回备选收费详情，取值0或1
	Extensions     string     // 可选，返回结果控制，base(默认)或all
}

// 货车路径规划响应结构
type V5TruckResponse struct {
	V5BaseResponse
	Route V5TruckRouteResult `json:"route"`
}

// 货车路径规划结果
type V5TruckRouteResult struct {
	Origin      string        `json:"origin"`      // 起点坐标
	Destination string        `json:"destination"` // 终点坐标
	TaxiCost    string        `json:"taxi_cost"`   // 出租车费用，单位：元
	Paths       []V5TruckPath `json:"paths"`       // 货车方案列表
}

// 货车路径规划方案
type V5TruckPath struct {
	Distance      string        `json:"distance"`       // 方案距离，单位：米
	Duration      string        `json:"duration"`       // 预计行驶时间，单位：秒
	Strategy      string        `json:"strategy"`       // 导航策略
	Tolls         string        `json:"tolls"`          // 此方案收费，单位：元
	TollDistance  string        `json:"toll_distance"`  // 收费路段长度，单位：米
	TrafficLights string        `json:"traffic_lights"` // 红绿灯个数
	Steps         []V5TruckStep `json:"steps"`          // 导航路段列表
	Restriction   string        `json:"restriction"`    // 限行结果
	Cost          *V5Cost       `json:"cost,omitempty"` // 方案花费
}

// 货车路径规划导航步骤
type V5TruckStep struct {
	Instruction     string `json:"instruction"`                // 行走指示
	Road            string `json:"road"`                       // 道路名称
	Distance        string `json:"distance"`                   // 此路段距离，单位：米
	Duration        string `json:"duration"`                   // 此路段预计耗时，单位：秒
	Polyline        string `json:"polyline"`                   // 此路段坐标点串
	Action          string `json:"action,omitempty"`           // 导航主要动作指令
	AssistantAction string `json:"assistant_action,omitempty"` // 导航辅助动作指令
	TollRoad        string `json:"toll_road"`                  // 收费道路
	TollCost        string `json:"toll_cost"`                  // 收费金额
}

// 货车路径规划
func (c *Client) V5Truck(ctx context.Context, request V5TruckRequest) (*V5TruckResponse, error) {
	params := url.Values{}
	params.Set("origin", request.Origin.String())
	params.Set("destination", request.Destination.String())

	if request.Strategy != 0 {
		params.Set("strategy", fmt.Sprintf("%d", request.Strategy))
	}

	if len(request.Waypoints) > 0 {
		waypoints := make([]string, len(request.Waypoints))
		for i, wp := range request.Waypoints {
			waypoints[i] = wp.String()
		}
		params.Set("waypoints", strings.Join(waypoints, ";"))
	}

	if len(request.ShowFields) > 0 {
		params.Set("show_fields", strings.Join(request.ShowFields, ","))
	}

	if len(request.AvoidRoad) > 0 {
		params.Set("avoid_road", strings.Join(request.AvoidRoad, ","))
	}

	if request.AvoidPoly != "" {
		params.Set("avoid_polygons", request.AvoidPoly)
	}

	if request.ProvinceCross {
		params.Set("province_cross", "1")
	}

	if request.RoadNetworkOpt > 0 {
		params.Set("road_network_opt", fmt.Sprintf("%d", request.RoadNetworkOpt))
	}

	if request.Height > 0 {
		params.Set("height", fmt.Sprintf("%.1f", request.Height))
	}

	if request.Width > 0 {
		params.Set("width", fmt.Sprintf("%.1f", request.Width))
	}

	if request.Length > 0 {
		params.Set("length", fmt.Sprintf("%.1f", request.Length))
	}

	if request.Weight > 0 {
		params.Set("weight", fmt.Sprintf("%.1f", request.Weight))
	}

	if request.Load > 0 {
		params.Set("load", fmt.Sprintf("%.1f", request.Load))
	}

	if request.AxisNum > 0 {
		params.Set("axis_num", fmt.Sprintf("%d", request.AxisNum))
	}

	if request.HazardType != "" {
		params.Set("hazard_type", request.HazardType)
	}

	if request.PlateNumber != "" {
		params.Set("plate_number", request.PlateNumber)
	}

	if request.PlateProvince != "" {
		params.Set("plate_province", request.PlateProvince)
	}

	if request.FerryType > 0 {
		params.Set("ferry_type", fmt.Sprintf("%d", request.FerryType))
	}

	if request.ExpandPath > 0 {
		params.Set("expand_path", fmt.Sprintf("%d", request.ExpandPath))
	}

	if request.PriceExpand > 0 {
		params.Set("price_expand", fmt.Sprintf("%d", request.PriceExpand))
	}

	if request.Extensions != "" {
		params.Set("extensions", request.Extensions)
	}

	response := &V5TruckResponse{}
	err := c.doRequest(ctx, "/v5/direction/truck", params, response)
	return response, err
}
