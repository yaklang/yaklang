package amap

var YakExport = map[string]any{
	// 地理位置获取
	"GetGeocode":        Geocode,
	"GetReverseGeocode": ReverseGeocode,
	// 路径规划
	"GetDrivingPlan":   DrivingPlan,
	"GetWalkingPlan":   WalkingPlan,
	"GetTransitPlan":   TransitPlan,
	"GetBicyclingPlan": BicyclingPlan,
	// 距离计算
	"GetDistance": Distance,
	// IP地理位置获取
	"GetIpLocation": IPLocation,
	// POI搜索
	"GetPOI":       SearchPOI,
	"GetNearbyPOI": SearchNearbyPOI,
	"GetPOIDetail": GetPOIDetail,
	// "GetAOIBoundary": _GetAOIBoundary,
	// 天气查询
	"GetWeather": GetWeather,

	// 配置选项
	"geocodeFilter": WithGeocodeFilter,
	"apiKey":        WithApiKey,
	"timeout":       WithTimeout,
	"baseURL":       WithBaseURL,
	"city":          WithCity,
	"extensions":    WithExtensions,
	"page":          WithPage,
	"pageSize":      WithPageSize,
	"type":          WithType,
	"radius":        WithRadius,
	"sortRule":      WithSortRule,
	"pocOpts":       WithLowhttpOptions,
}
