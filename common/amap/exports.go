package amap

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

// IPLocationResultEx contains IP geolocation information
type IPLocationResultEx struct {
	IP        string
	Province  string
	City      string
	AdCode    string
	Rectangle string
}

// POIResultEx contains simplified POI information
type POIResultEx struct {
	Name         string
	Address      string
	Location     Location
	Distance     string
	Type         string
	TelNumber    string
	Province     string
	City         string
	District     string
	POIId        string
	AdCode       string
	BusinessArea string
}

// SearchPOIResultEx contains search result information
type SearchPOIResultEx struct {
	Total       int
	Results     []POIResultEx
	SugKeywords []string
	SugCities   []string
}

// AOIBoundaryResultEx contains AOI boundary information
type AOIBoundaryResultEx struct {
	Name       string
	ID         string
	Type       string
	Address    string
	Location   Location
	Province   string
	City       string
	District   string
	AdCode     string
	Polyline   string     // Boundary coordinates
	PolyPoints []Location // Parsed boundary points
}

// Geocode 将地址转换为经纬度坐标（地理编码，导出名为 amap.GetGeocode）
// 参数:
//   - address: 结构化地址描述
//   - options: 可选项，需要 amap.apiKey 提供高德 API Key
//
// 返回值:
//   - 地理编码结果列表
//   - 错误信息
//
// Example:
// ```
// // 需要有效的高德 API Key（示意性示例）
// results = amap.GetGeocode("北京市朝阳区阜通东大街6号", amap.apiKey("your-key"))~
// dump(results)
// ```
func Geocode(address string, options ...AmapConfigOption) ([]*GeocodeResult, error) {
	config := NewConfig(options...)
	ctx, cancel := context.WithTimeout(context.Background(), config.Timeout)
	defer cancel()

	client, err := NewClientByConfig(config)
	if err != nil {
		return nil, err
	}

	req := GeocodingRequest{
		Address: address,
	}

	if config.City != "" {
		req.City = config.City
	}

	resp, err := client.Geocode(ctx, req)
	if err != nil {
		return nil, err
	}

	if len(resp.Geocodes) == 0 {
		return nil, fmt.Errorf("no geocode results found")
	}
	return resp.Geocodes, nil
}

// ReverseGeocode 将经纬度坐标转换为结构化地址（逆地理编码，导出名为 amap.GetReverseGeocode）
// 参数:
//   - longitude: 经度
//   - latitude: 纬度
//   - options: 可选项，需要 amap.apiKey 提供高德 API Key
//
// 返回值:
//   - 逆地理编码结果
//   - 错误信息
//
// Example:
// ```
// // 需要有效的高德 API Key（示意性示例）
// result = amap.GetReverseGeocode(116.481488, 39.990464, amap.apiKey("your-key"))~
// dump(result)
// ```
func ReverseGeocode(longitude, latitude float64, options ...AmapConfigOption) (*RegeoCodeResult, error) {
	config := NewConfig(options...)

	ctx, cancel := context.WithTimeout(context.Background(), config.Timeout)
	defer cancel()

	client, err := NewClientByConfig(config)
	if err != nil {
		return nil, err
	}

	location := Location{Longitude: longitude, Latitude: latitude}
	req := ReverseGeocodingRequest{
		Location: location,
	}

	if config.Extensions != "" {
		req.Extensions = config.Extensions
	}

	resp, err := client.ReverseGeocode(ctx, req)
	if err != nil {
		return nil, err
	}

	if resp.RegeoCodes == nil {
		return nil, fmt.Errorf("no reverse geocode results found")
	}

	return resp.RegeoCodes, nil
}

// DrivingPlan 计算两地之间的驾车路径规划（导出名为 amap.GetDrivingPlan）
// 参数:
//   - origin: 起点地址
//   - destination: 终点地址
//   - options: 可选项，需要 amap.apiKey 提供高德 API Key
//
// 返回值:
//   - 驾车路径规划结果
//   - 错误信息
//
// Example:
// ```
// // 需要有效的高德 API Key（示意性示例）
// plan = amap.GetDrivingPlan("北京站", "北京西站", amap.apiKey("your-key"))~
// dump(plan)
// ```
func DrivingPlan(origin, destination string, options ...AmapConfigOption) (*DirectionResponse, error) {
	originGeocodes, err := Geocode(origin, options...)
	if err != nil {
		return nil, err
	}
	destinationGeocodes, err := Geocode(destination, options...)
	if err != nil {
		return nil, err
	}

	config := NewConfig(options...)
	originGeocode := config.GeocodeFilter(originGeocodes)
	destinationGeocode := config.GeocodeFilter(destinationGeocodes)
	originLocation, err := LocationFromString(originGeocode.Location)
	if err != nil {
		return nil, err
	}
	destinationLocation, err := LocationFromString(destinationGeocode.Location)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), config.Timeout)
	defer cancel()

	client, err := NewClientByConfig(config)
	if err != nil {
		return nil, err
	}

	req := DrivingRequest{
		DirectionRequest: DirectionRequest{
			Origin:      originLocation,
			Destination: destinationLocation,
		},
		Strategy: RouteStrategyFastest,
	}

	if config.Extensions != "" {
		req.OutputExtensions = config.Extensions
	}

	return client.Driving(ctx, req)
}

// WalkingPlan 计算两地之间的步行路径规划（导出名为 amap.GetWalkingPlan）
// 参数:
//   - origin: 起点地址
//   - destination: 终点地址
//   - options: 可选项，需要 amap.apiKey 提供高德 API Key
//
// 返回值:
//   - 步行路径规划结果
//   - 错误信息
//
// Example:
// ```
// // 需要有效的高德 API Key（示意性示例）
// plan = amap.GetWalkingPlan("北京站", "天安门", amap.apiKey("your-key"))~
// dump(plan)
// ```
func WalkingPlan(origin, destination string, options ...AmapConfigOption) (*V5WalkingResponse, error) {
	originGeocodes, err := Geocode(origin, options...)
	if err != nil {
		return nil, err
	}
	destinationGeocodes, err := Geocode(destination, options...)
	if err != nil {
		return nil, err
	}

	config := NewConfig(options...)
	originGeocode := config.GeocodeFilter(originGeocodes)
	destinationGeocode := config.GeocodeFilter(destinationGeocodes)
	originLocation, err := LocationFromString(originGeocode.Location)
	if err != nil {
		return nil, err
	}
	destinationLocation, err := LocationFromString(destinationGeocode.Location)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), config.Timeout)
	defer cancel()

	client, err := NewClientByConfig(config)
	if err != nil {
		return nil, err
	}

	req := V5WalkingRequest{
		Origin:      originLocation,
		Destination: destinationLocation,
	}

	if config.Extensions != "" {
		req.Extensions = config.Extensions
	}

	return client.V5Walking(ctx, req)
}

// BicyclingPlan 计算两地之间的骑行路径规划（导出名为 amap.GetBicyclingPlan）
// 参数:
//   - origin: 起点地址
//   - destination: 终点地址
//   - options: 可选项，需要 amap.apiKey 提供高德 API Key
//
// 返回值:
//   - 骑行路径规划结果
//   - 错误信息
//
// Example:
// ```
// // 需要有效的高德 API Key（示意性示例）
// plan = amap.GetBicyclingPlan("北京站", "天安门", amap.apiKey("your-key"))~
// dump(plan)
// ```
func BicyclingPlan(origin, destination string, options ...AmapConfigOption) (*BicyclingResult, error) {
	originGeocodes, err := Geocode(origin, options...)
	if err != nil {
		return nil, err
	}
	destinationGeocodes, err := Geocode(destination, options...)
	if err != nil {
		return nil, err
	}

	config := NewConfig(options...)
	originGeocode := config.GeocodeFilter(originGeocodes)
	destinationGeocode := config.GeocodeFilter(destinationGeocodes)
	originLocation, err := LocationFromString(originGeocode.Location)
	if err != nil {
		return nil, err
	}
	destinationLocation, err := LocationFromString(destinationGeocode.Location)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), config.Timeout)
	defer cancel()

	client, err := NewClientByConfig(config)
	if err != nil {
		return nil, err
	}

	req := BicyclingRequest{
		DirectionRequest: DirectionRequest{
			Origin:      originLocation,
			Destination: destinationLocation,
		},
	}

	if config.Extensions != "" {
		req.OutputExtensions = config.Extensions
	}

	resp, err := client.Bicycling(ctx, req)
	if err != nil {
		return nil, err
	}

	return &resp.Data, nil
}

// TransitPlan 计算两地之间的公交路径规划（导出名为 amap.GetTransitPlan）
// 参数:
//   - origin: 起点地址
//   - destination: 终点地址
//   - options: 可选项，需要 amap.apiKey 提供高德 API Key，公交规划通常还需 amap.city
//
// 返回值:
//   - 公交路径规划结果
//   - 错误信息
//
// Example:
// ```
// // 需要有效的高德 API Key（示意性示例）
// plan = amap.GetTransitPlan("北京站", "北京西站", amap.apiKey("your-key"), amap.city("北京"))~
// dump(plan)
// ```
func TransitPlan(origin, destination string, options ...AmapConfigOption) (*TransitResponse, error) {
	originGeocodes, err := Geocode(origin, options...)
	if err != nil {
		return nil, err
	}
	destinationGeocodes, err := Geocode(destination, options...)
	if err != nil {
		return nil, err
	}

	config := NewConfig(options...)
	originGeocode := config.GeocodeFilter(originGeocodes)
	destinationGeocode := config.GeocodeFilter(destinationGeocodes)
	originLocation, err := LocationFromString(originGeocode.Location)
	if err != nil {
		return nil, err
	}
	destinationLocation, err := LocationFromString(destinationGeocode.Location)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), config.Timeout)
	defer cancel()

	client, err := NewClientByConfig(config)
	if err != nil {
		return nil, err
	}

	req := TransitRequest{
		DirectionRequest: DirectionRequest{
			Origin:      originLocation,
			Destination: destinationLocation,
		},
		City:   originGeocode.CityCode,
		CitydD: destinationGeocode.CityCode,
	}

	if config.Extensions != "" {
		req.OutputExtensions = config.Extensions
	}

	return client.Transit(ctx, req)
}

// Distance 计算两地之间的距离（导出名为 amap.GetDistance）
// 参数:
//   - origin: 起点地址
//   - destination: 终点地址
//   - options: 可选项，需要 amap.apiKey 提供高德 API Key，可用 amap.type 指定测距方式
//
// 返回值:
//   - 距离计算结果
//   - 错误信息
//
// Example:
// ```
// // 需要有效的高德 API Key（示意性示例）
// dist = amap.GetDistance("北京站", "北京西站", amap.apiKey("your-key"))~
// dump(dist)
// ```
func Distance(origin, destination string, options ...AmapConfigOption) (*DistanceResult, error) {
	config := NewConfig(options...)

	ctx, cancel := context.WithTimeout(context.Background(), config.Timeout)
	defer cancel()

	client, err := NewClientByConfig(config)
	if err != nil {
		return nil, err
	}

	originGeocodes, err := Geocode(origin, options...)
	if err != nil {
		return nil, err
	}

	originGeocode := config.GeocodeFilter(originGeocodes)
	originLocation, err := LocationFromString(originGeocode.Location)
	if err != nil {
		return nil, err
	}
	destinationGeocodes, err := Geocode(destination, options...)
	if err != nil {
		return nil, err
	}
	destinationGeocode := config.GeocodeFilter(destinationGeocodes)
	destinationLocation, err := LocationFromString(destinationGeocode.Location)
	if err != nil {
		return nil, err
	}
	req := DistanceRequest{
		Origins:     []Location{originLocation},
		Destination: destinationLocation,
		Type:        DirectionType(config.Type),
	}
	distance, err := client.Distance(ctx, req)
	if err != nil {
		return nil, err
	}
	if len(distance.Results) == 0 {
		return nil, fmt.Errorf("no distance results found")
	}
	return &distance.Results[0], nil
}

// IPLocation 根据 IP 地址定位其地理位置（ip 为空时定位请求方 IP，导出名为 amap.GetIpLocation）
// 参数:
//   - ip: 待定位的 IP 地址，为空时定位请求方 IP
//   - options: 可选项，需要 amap.apiKey 提供高德 API Key
//
// 返回值:
//   - IP 地理位置结果
//   - 错误信息
//
// Example:
// ```
// // 需要有效的高德 API Key（示意性示例）
// loc = amap.GetIpLocation("114.114.114.114", amap.apiKey("your-key"))~
// dump(loc)
// ```
func IPLocation(ip string, options ...AmapConfigOption) (*IPLocationResultEx, error) {
	config := NewConfig(options...)

	ctx, cancel := context.WithTimeout(context.Background(), config.Timeout)
	defer cancel()

	client, err := NewClientByConfig(config)
	if err != nil {
		return nil, err
	}

	resp, err := client.IPLocationV3(ctx, IPLocationRequest{
		IP: ip,
	})
	if err != nil {
		return nil, err
	}

	return &IPLocationResultEx{
		IP:        ip,
		Province:  resp.Province,
		City:      resp.City,
		AdCode:    resp.AdCode,
		Rectangle: resp.Rectangle,
	}, nil
}

// SearchPOI 基于关键词搜索兴趣点（POI，导出名为 amap.GetPOI）
// 参数:
//   - keywords: 搜索关键词
//   - options: 可选项，需要 amap.apiKey 提供高德 API Key，可用 amap.city、amap.page、amap.pageSize
//
// 返回值:
//   - POI 搜索结果
//   - 错误信息
//
// Example:
// ```
// // 需要有效的高德 API Key（示意性示例）
// pois = amap.GetPOI("咖啡", amap.apiKey("your-key"), amap.city("北京"))~
// dump(pois)
// ```
func SearchPOI(keywords string, options ...AmapConfigOption) (*SearchPOIResultEx, error) {
	config := NewConfig(options...)

	ctx, cancel := context.WithTimeout(context.Background(), config.Timeout)
	defer cancel()

	client, err := NewClientByConfig(config)
	if err != nil {
		return nil, err
	}

	req := SearchPOIRequest{
		Keywords:   keywords,
		Page:       config.Page,
		Offset:     config.PageSize,
		Extensions: config.Extensions,
		City:       config.City,
		CityLimit:  true,
	}

	if config.City != "" {
		req.City = config.City
	}

	resp, err := client.SearchPOI(ctx, req)
	if err != nil {
		return nil, err
	}

	total, _ := strconv.Atoi(resp.Count)
	result := &SearchPOIResultEx{
		Total:       total,
		Results:     make([]POIResultEx, 0, len(resp.Pois)),
		SugKeywords: resp.Suggestion.Keywords,
		SugCities:   resp.Suggestion.Cities,
	}

	for _, poi := range resp.Pois {
		location, _ := LocationFromString(poi.Location)
		result.Results = append(result.Results, POIResultEx{
			Name:         poi.Name,
			Address:      poi.Address,
			Location:     location,
			Distance:     poi.Distance,
			Type:         poi.Type,
			TelNumber:    poi.Tel,
			POIId:        poi.ID,
			BusinessArea: poi.Businessarea,
		})
	}

	return result, nil
}

// SearchNearbyPOI 基于坐标的周边兴趣点搜索（导出名为 amap.GetNearbyPOI）
// 参数:
//   - location: 中心点坐标（如 "116.481,39.990"）
//   - keywords: 搜索关键词
//   - options: 可选项，需要 amap.apiKey 提供高德 API Key，可用 amap.radius 指定半径
//
// 返回值:
//   - POI 搜索结果
//   - 错误信息
//
// Example:
// ```
// // 需要有效的高德 API Key（示意性示例）
// pois = amap.GetNearbyPOI("116.481488,39.990464", "咖啡", amap.apiKey("your-key"), amap.radius(1000))~
// dump(pois)
// ```
func SearchNearbyPOI(location string, keywords string, options ...AmapConfigOption) (*SearchPOIResultEx, error) {
	config := NewConfig(options...)

	ctx, cancel := context.WithTimeout(context.Background(), config.Timeout)
	defer cancel()

	client, err := NewClientByConfig(config)
	if err != nil {
		return nil, err
	}

	locationGeocodes, err := Geocode(location, options...)
	if err != nil {
		return nil, err
	}
	locationGeocode := config.GeocodeFilter(locationGeocodes)
	locationLocation, err := LocationFromString(locationGeocode.Location)
	if err != nil {
		return nil, err
	}
	req := SearchNearbyPOIRequest{
		Location:   locationLocation,
		Keywords:   keywords,
		Radius:     config.Radius,
		SortRule:   config.SortRule,
		Extensions: config.Extensions,
		Page:       config.Page,
		Offset:     config.PageSize,
		City:       config.City,
	}

	if config.City != "" {
		req.City = config.City
	}

	resp, err := client.SearchNearbyPOI(ctx, req)
	if err != nil {
		return nil, err
	}

	total, _ := strconv.Atoi(resp.Count)
	result := &SearchPOIResultEx{
		Total:       total,
		Results:     make([]POIResultEx, 0, len(resp.Pois)),
		SugKeywords: resp.Suggestion.Keywords,
		SugCities:   resp.Suggestion.Cities,
	}

	for _, poi := range resp.Pois {
		location, _ := LocationFromString(poi.Location)
		result.Results = append(result.Results, POIResultEx{
			Name:         poi.Name,
			Address:      poi.Address,
			Location:     location,
			Distance:     poi.Distance,
			Type:         poi.Type,
			TelNumber:    poi.Tel,
			POIId:        poi.ID,
			BusinessArea: poi.Businessarea,
		})
	}

	return result, nil
}

// GetPOIDetail 根据 POI ID 查询兴趣点详情（导出名为 amap.GetPOIDetail）
// 参数:
//   - poiID: 兴趣点 ID
//   - options: 可选项，需要 amap.apiKey 提供高德 API Key
//
// 返回值:
//   - POI 详情结果
//   - 错误信息
//
// Example:
// ```
// // 需要有效的高德 API Key（示意性示例）
// detail = amap.GetPOIDetail("B000A83M61", amap.apiKey("your-key"))~
// dump(detail)
// ```
func GetPOIDetail(poiID string, options ...AmapConfigOption) (*POIResultEx, error) {
	config := NewConfig(options...)

	ctx, cancel := context.WithTimeout(context.Background(), config.Timeout)
	defer cancel()

	client, err := NewClientByConfig(config)
	if err != nil {
		return nil, err
	}

	resp, err := client.SearchPOIByID(ctx, SearchPOIByIDRequest{
		ID:         poiID,
		Extensions: config.Extensions,
	})
	if err != nil {
		return nil, err
	}

	if len(resp.Pois) == 0 {
		return nil, fmt.Errorf("POI not found: %s", poiID)
	}

	poi := resp.Pois[0]
	location, _ := LocationFromString(poi.Location)

	return &POIResultEx{
		Name:         poi.Name,
		Address:      poi.Address,
		Location:     location,
		Distance:     poi.Distance,
		Type:         poi.Type,
		TelNumber:    poi.Tel,
		POIId:        poi.ID,
		BusinessArea: poi.Businessarea,
	}, nil
}

// GetAOIBoundary retrieves boundary information for an Area of Interest
func _GetAOIBoundary(aoiID string, options ...AmapConfigOption) (*AOIBoundaryResultEx, error) {
	config := NewConfig(options...)

	ctx, cancel := context.WithTimeout(context.Background(), config.Timeout)
	defer cancel()

	client, err := NewClientByConfig(config)
	if err != nil {
		return nil, err
	}

	resp, err := client.AOIBoundary(ctx, AOIBoundaryRequest{
		ID: aoiID,
	})
	if err != nil {
		return nil, err
	}

	if len(resp.Aois) == 0 {
		return nil, fmt.Errorf("AOI not found: %s", aoiID)
	}

	aoi := resp.Aois[0]
	location, _ := LocationFromString(aoi.Location)

	// Process the polyline
	polyPoints := make([]Location, 0)
	if aoi.Polyline != "" {
		parts := strings.Split(strings.Replace(aoi.Polyline, "_", ";", -1), ";")
		for _, part := range parts {
			if loc, err := LocationFromString(part); err == nil {
				polyPoints = append(polyPoints, loc)
			}
		}
	}

	return &AOIBoundaryResultEx{
		Name:       aoi.Name,
		ID:         aoi.ID,
		Type:       aoi.Type,
		Address:    aoi.Address,
		Location:   location,
		Province:   aoi.PName,
		City:       aoi.CityName,
		District:   aoi.AdName,
		AdCode:     aoi.AdCode,
		Polyline:   aoi.Polyline,
		PolyPoints: polyPoints,
	}, nil
}

// WeatherResultEx contains simplified weather information
type WeatherResultEx struct {
	City          string // City name
	AdCode        string // Administrative area code
	Province      string // Province name
	Weather       string // Current weather condition
	Temperature   string // Current temperature in Celsius
	WindDirection string // Wind direction
	WindPower     string // Wind power level
	Humidity      string // Humidity percentage
	ReportTime    string // Report time

	// Forecast data (when available)
	Forecast []WeatherForecastEx
}

// WeatherForecastEx contains simplified forecast information
type WeatherForecastEx struct {
	Date         string // Date (YYYY-MM-DD)
	Week         string // Day of week
	DayWeather   string // Day weather condition
	NightWeather string // Night weather condition
	DayTemp      string // Day temperature in Celsius
	NightTemp    string // Night temperature in Celsius
	DayWind      string // Day wind direction
	NightWind    string // Night wind direction
}

// GetWeather 查询指定城市的天气信息（导出名为 amap.GetWeather）
// 参数:
//   - cityCode: 城市编码（adcode）
//   - options: 可选项，需要 amap.apiKey 提供高德 API Key，可用 amap.enableWeatherForecast 返回预报
//
// 返回值:
//   - 天气查询结果
//   - 错误信息
//
// Example:
// ```
// // 需要有效的高德 API Key（示意性示例）
// weather = amap.GetWeather("110000", amap.apiKey("your-key"))~
// dump(weather)
// ```
func GetWeather(cityCode string, options ...AmapConfigOption) (*WeatherResponse, error) {
	config := NewConfig(options...)

	ctx, cancel := context.WithTimeout(context.Background(), config.Timeout)
	defer cancel()

	client, err := NewClientByConfig(config)
	if err != nil {
		return nil, err
	}

	extensions := WeatherTypeBase
	if config.EnableWeatherForecast {
		extensions = WeatherTypeAll
	}

	return client.Weather(ctx, WeatherRequest{
		City:       cityCode,
		Extensions: extensions,
	})
}
