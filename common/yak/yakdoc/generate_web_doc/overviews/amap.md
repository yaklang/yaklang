`amap` 库是高德地图（AMap）开放平台 API 的封装，提供地理编码、逆地理编码、POI 检索、路径规划、IP 定位、天气查询等地理信息服务能力。使用前需要申请高德 API Key，通过 `amap.apiKey` 传入。

典型使用场景：

- 地理编码：`amap.GetGeocode`（地址转坐标）、`amap.GetReverseGeocode`（坐标转地址）、`amap.GetIpLocation`（IP 定位）。
- 地点检索：`amap.GetPOI` / `amap.GetNearbyPOI` / `amap.GetPOIDetail` 搜索兴趣点；`amap.GetDistance` 测算距离。
- 路径规划：`amap.GetDrivingPlan` / `amap.GetWalkingPlan` / `amap.GetBicyclingPlan` / `amap.GetTransitPlan` 分别规划驾车、步行、骑行与公交路线。
- 其他：`amap.GetWeather` 查询天气；通过 `amap.city` / `amap.radius` / `amap.page` / `amap.pageSize` / `amap.timeout` 等选项细化查询，`amap.pocOpts` 透传底层 HTTP 选项（如代理）。

与相邻库的关系：`amap` 底层基于 `poc`/HTTP 请求实现，是面向外部数据源（地理信息）的便捷封装，常用于资产地理画像、目标定位等场景。
