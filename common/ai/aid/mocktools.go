package aid

import (
	"fmt"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"io"
	"math/rand"
	"strings"
	"time"
)

// 初始化随机数生成器
func init() {
	rand.Seed(time.Now().UnixNano())
}

// 提供所有Mock工具的集合
func GetAllMockTools() []*aitool.Tool {
	return []*aitool.Tool{
		WeatherTool(),
		AttractionTool(),
		RestaurantTool(),
		TransportTool(),
		TimeEstimateTool(),
	}
}

// WeatherTool 创建天气查询工具
func WeatherTool() *aitool.Tool {
	callback := func(params aitool.InvokeParams, stdout io.Writer, stderr io.Writer) (interface{}, error) {
		city, _ := params["city"].(string)
		date, _ := params["date"].(string)

		fmt.Fprintf(stdout, "正在查询 %s 在 %s 的天气...\n", city, date)

		// 模拟网络延迟
		time.Sleep(time.Duration(rand.Intn(500)) * time.Millisecond)

		// 随机决定返回结果类型
		resultType := rand.Intn(10)
		if resultType < 1 { // 10%概率返回错误
			fmt.Fprintf(stderr, "获取天气数据失败\n")
			return map[string]interface{}{
				"status":  "error",
				"message": "无法获取天气数据，请稍后再试",
			}, nil
		}

		// 随机天气情况
		var result map[string]interface{}
		if resultType < 7 { // 60%概率是好天气
			result = map[string]interface{}{
				"status": "success",
				"data": map[string]interface{}{
					"city":           city,
					"date":           date,
					"weather":        "晴转多云",
					"temperature":    "15°C - 24°C",
					"precipitation":  "10%",
					"recommendation": "适合户外活动",
				},
			}
		} else { // 30%概率是坏天气
			result = map[string]interface{}{
				"status": "success",
				"data": map[string]interface{}{
					"city":           city,
					"date":           date,
					"weather":        "小雨",
					"temperature":    "8°C - 14°C",
					"precipitation":  "80%",
					"recommendation": "不建议进行长时间户外活动",
				},
			}
		}

		return result, nil
	}

	// 创建天气工具
	tool, _ := aitool.New("WeatherAPI",
		aitool.WithDescription("天气查询工具 - 根据城市和日期查询天气情况"),
		aitool.WithSimpleCallback(callback),
		aitool.WithStringParam("city",
			aitool.WithParam_Description("城市名称"),
			aitool.WithParam_Required(),
		),
		aitool.WithStringParam("date",
			aitool.WithParam_Description("查询日期 (YYYY-MM-DD格式)"),
			aitool.WithParam_Required(),
		),
	)

	return tool
}

// AttractionTool 创建景点推荐工具
func AttractionTool() *aitool.Tool {
	callback := func(params aitool.InvokeParams, stdout io.Writer, stderr io.Writer) (interface{}, error) {
		city, _ := params["city"].(string)
		preference, _ := params["preference"].(string)

		fmt.Fprintf(stdout, "正在查询 %s 的%s类型景点...\n", city, preference)

		// 模拟网络延迟
		time.Sleep(time.Duration(rand.Intn(800)) * time.Millisecond)

		// 随机决定返回结果类型
		if rand.Intn(10) < 2 { // 20%概率部分景点关闭
			return map[string]interface{}{
				"status":  "partial",
				"message": "部分景点暂时关闭或有特殊安排",
				"data": []map[string]interface{}{
					{
						"name":       "故宫博物院",
						"type":       "历史文化",
						"status":     "闭馆维修",
						"reopenDate": "2025-03-20",
					},
					{
						"name":                "颐和园",
						"type":                "历史文化/自然",
						"openTime":            "6:30-18:00",
						"ticketPrice":         "成人60元，儿童30元",
						"recommendedDuration": "2-3小时",
						"crowdLevel":          "中",
						"coordinates":         "39.999741,116.275626",
					},
				},
			}, nil
		}

		// 正常返回景点列表
		attractions := []map[string]interface{}{
			{
				"name":                "故宫博物院",
				"type":                "历史文化",
				"openTime":            "8:30-17:00",
				"ticketPrice":         "成人120元，儿童60元",
				"recommendedDuration": "3-4小时",
				"crowdLevel":          "高",
				"coordinates":         "39.916345,116.397155",
			},
			{
				"name":                "颐和园",
				"type":                "历史文化/自然",
				"openTime":            "6:30-18:00",
				"ticketPrice":         "成人60元，儿童30元",
				"recommendedDuration": "2-3小时",
				"crowdLevel":          "中",
				"coordinates":         "39.999741,116.275626",
			},
			{
				"name":                "北京动物园",
				"type":                "娱乐/自然",
				"openTime":            "7:30-17:00",
				"ticketPrice":         "成人15元，儿童8元",
				"recommendedDuration": "2-3小时",
				"crowdLevel":          "高",
				"coordinates":         "39.942845,116.339390",
			},
		}

		// 如果有偏好，筛选符合偏好的景点
		if preference != "" {
			filteredAttractions := []map[string]interface{}{}
			for _, attraction := range attractions {
				attrType, _ := attraction["type"].(string)
				if strings.Contains(strings.ToLower(attrType), strings.ToLower(preference)) {
					filteredAttractions = append(filteredAttractions, attraction)
				}
			}

			// 如果没有匹配的景点，返回原始列表
			if len(filteredAttractions) > 0 {
				attractions = filteredAttractions
			} else {
				fmt.Fprintf(stderr, "没有找到完全匹配偏好的景点，返回所有景点\n")
			}
		}

		return map[string]interface{}{
			"status": "success",
			"data":   attractions,
		}, nil
	}

	// 创建景点工具
	tool, _ := aitool.New("AttractionAPI",
		aitool.WithDescription("景点推荐工具 - 根据城市和偏好类型推荐景点"),
		aitool.WithSimpleCallback(callback),
		aitool.WithStringParam("city",
			aitool.WithParam_Description("城市名称"),
			aitool.WithParam_Required(),
		),
		aitool.WithStringParam("preference",
			aitool.WithParam_EnumString("历史", "自然", "文化", "娱乐"),
			aitool.WithParam_Description("偏好类型"),
		),
	)

	return tool
}

// RestaurantTool 创建餐厅搜索工具
func RestaurantTool() *aitool.Tool {
	callback := func(params aitool.InvokeParams, stdout io.Writer, stderr io.Writer) (interface{}, error) {
		location, _ := params["location"].(string)
		budget, _ := params["budget"].(string)
		cuisine, _ := params["cuisine"].(string)

		fmt.Fprintf(stdout, "正在搜索 %s 区域的%s餐厅...\n", location, cuisine)
		if budget != "" {
			fmt.Fprintf(stdout, "预算范围: %s\n", budget)
		}

		// 模拟网络延迟
		time.Sleep(time.Duration(rand.Intn(700)) * time.Millisecond)

		// 生成模拟餐厅数据
		restaurants := []map[string]interface{}{
			{
				"name":       "北京烤鸭店",
				"rating":     4.8,
				"priceRange": "¥¥¥",
				"avgPrice":   188,
				"cuisine":    "中餐",
				"openHours":  "10:00-22:00",
				"address":    location + " 中心广场123号",
				"distance":   "1.2公里",
			},
			{
				"name":       "和风寿司",
				"rating":     4.5,
				"priceRange": "¥¥¥¥",
				"avgPrice":   258,
				"cuisine":    "日料",
				"openHours":  "11:00-21:30",
				"address":    location + " 商业街45号",
				"distance":   "0.8公里",
			},
			{
				"name":       "老四川火锅",
				"rating":     4.2,
				"priceRange": "¥¥",
				"avgPrice":   128,
				"cuisine":    "川菜",
				"openHours":  "11:00-23:30",
				"address":    location + " 美食城78号",
				"distance":   "1.5公里",
			},
		}

		// 根据菜系筛选
		if cuisine != "" {
			filtered := []map[string]interface{}{}
			for _, r := range restaurants {
				if strings.Contains(r["cuisine"].(string), cuisine) {
					filtered = append(filtered, r)
				}
			}

			if len(filtered) > 0 {
				restaurants = filtered
			} else {
				fmt.Fprintf(stderr, "未找到符合菜系要求的餐厅，返回所有餐厅\n")
			}
		}

		// 根据预算筛选
		if budget != "" {
			budgetMatched := []map[string]interface{}{}
			// 简单解析预算范围
			var priceSymbols string
			if strings.Contains(budget, "低") || strings.Contains(budget, "经济") {
				priceSymbols = "¥"
			} else if strings.Contains(budget, "中") {
				priceSymbols = "¥¥"
			} else if strings.Contains(budget, "高") || strings.Contains(budget, "奢侈") {
				priceSymbols = "¥¥¥¥"
			}

			if priceSymbols != "" {
				for _, r := range restaurants {
					if strings.HasPrefix(r["priceRange"].(string), priceSymbols) {
						budgetMatched = append(budgetMatched, r)
					}
				}

				if len(budgetMatched) > 0 {
					restaurants = budgetMatched
				} else {
					fmt.Fprintf(stderr, "未找到符合预算要求的餐厅，返回所有餐厅\n")
				}
			}
		}

		return map[string]interface{}{
			"status": "success",
			"count":  len(restaurants),
			"data":   restaurants,
		}, nil
	}

	// 创建餐厅工具
	tool, _ := aitool.New("RestaurantAPI",
		aitool.WithDescription("餐厅搜索工具 - 根据位置、预算和菜系搜索餐厅"),
		aitool.WithSimpleCallback(callback),
		aitool.WithStringParam("location",
			aitool.WithParam_Description("位置坐标或区域名称"),
			aitool.WithParam_Required(),
		),
		aitool.WithStringParam("budget",
			aitool.WithParam_Description("预算范围"),
		),
		aitool.WithStringParam("cuisine",
			aitool.WithParam_Description("菜系偏好"),
		),
	)

	return tool
}

// TransportTool 创建交通查询工具
func TransportTool() *aitool.Tool {
	callback := func(params aitool.InvokeParams, stdout io.Writer, stderr io.Writer) (interface{}, error) {
		origin, _ := params["origin"].(string)
		destination, _ := params["destination"].(string)

		fmt.Fprintf(stdout, "正在查询从 %s 到 %s 的交通路线...\n", origin, destination)

		// 模拟网络延迟
		time.Sleep(time.Duration(rand.Intn(600)) * time.Millisecond)

		// 生成交通选项
		transportOptions := []map[string]interface{}{
			{
				"type":     "地铁",
				"route":    "1号线 → 3号线",
				"duration": "45分钟",
				"cost":     "4元",
				"details":  "步行10分钟到地铁站，换乘1次",
			},
			{
				"type":     "公交",
				"route":    "302路 → 513路",
				"duration": "65分钟",
				"cost":     "3元",
				"details":  "步行5分钟到公交站，换乘1次",
			},
			{
				"type":     "出租车",
				"route":    "直达",
				"duration": "25分钟",
				"cost":     "48元",
				"details":  "可能会遇到交通拥堵",
			},
			{
				"type":     "共享单车",
				"route":    "直达",
				"duration": "35分钟",
				"cost":     "2元",
				"details":  "有自行车道，比较安全",
			},
		}

		return map[string]interface{}{
			"status": "success",
			"count":  len(transportOptions),
			"data":   transportOptions,
		}, nil
	}

	// 创建交通工具
	tool, _ := aitool.New("TransportAPI",
		aitool.WithDescription("交通查询工具 - 查询两地之间的交通方式"),
		aitool.WithSimpleCallback(callback),
		aitool.WithStringParam("origin",
			aitool.WithParam_Description("起点位置"),
			aitool.WithParam_Required(),
		),
		aitool.WithStringParam("destination",
			aitool.WithParam_Description("终点位置"),
			aitool.WithParam_Required(),
		),
	)

	return tool
}

// TimeEstimateTool 创建行程时间估算工具
func TimeEstimateTool() *aitool.Tool {
	callback := func(params aitool.InvokeParams, stdout io.Writer, stderr io.Writer) (interface{}, error) {
		locationsObj, ok := params["locations"]
		if !ok {
			fmt.Fprintf(stderr, "地点列表不能为空\n")
			return map[string]interface{}{
				"status":  "error",
				"message": "请提供有效的地点列表",
			}, nil
		}

		locations, ok := locationsObj.([]interface{})
		if !ok || len(locations) == 0 {
			fmt.Fprintf(stderr, "地点列表不能为空\n")
			return map[string]interface{}{
				"status":  "error",
				"message": "请提供有效的地点列表",
			}, nil
		}

		durationsObj, _ := params["durations"]
		durations, _ := durationsObj.([]interface{})

		// 确保地点和停留时间数量匹配
		if len(durations) > 0 && len(durations) != len(locations) {
			fmt.Fprintf(stderr, "地点数量和停留时间数量不匹配\n")
			return map[string]interface{}{
				"status":  "error",
				"message": "地点数量和停留时间数量不匹配",
			}, nil
		}

		fmt.Fprintf(stdout, "正在估算行程时间...\n")
		fmt.Fprintf(stdout, "地点列表: %v\n", locations)

		// 如果没有提供停留时间，为每个地点设置默认停留时间（2小时）
		if len(durations) == 0 {
			durations = make([]interface{}, len(locations))
			for i := range durations {
				durations[i] = "2小时"
			}
		}

		// 模拟网络延迟
		time.Sleep(time.Duration(rand.Intn(800)) * time.Millisecond)

		// 生成行程计划
		itinerary := []map[string]interface{}{}
		totalMinutes := 0
		lastEndTime := 540 // 9:00开始，以分钟计

		for i, loc := range locations {
			location := loc.(string)
			durationStr := durations[i].(string)

			// 解析停留时间（简单实现，实际会更复杂）
			durationMinutes := 120 // 默认2小时
			if strings.Contains(durationStr, "小时") {
				hourStr := strings.Split(durationStr, "小时")[0]
				hours := 0
				fmt.Sscanf(hourStr, "%d", &hours)
				durationMinutes = hours * 60
			}

			// 计算交通时间（模拟）
			transitMinutes := 0
			if i > 0 {
				transitMinutes = 30 + rand.Intn(30) // 30-60分钟
			}

			startTime := lastEndTime + transitMinutes
			endTime := startTime + durationMinutes

			// 格式化时间
			startHour, startMin := startTime/60, startTime%60
			endHour, endMin := endTime/60, endTime%60

			itinerary = append(itinerary, map[string]interface{}{
				"location":      location,
				"arrivalTime":   fmt.Sprintf("%02d:%02d", startHour, startMin),
				"departureTime": fmt.Sprintf("%02d:%02d", endHour, endMin),
				"duration":      durationStr,
				"transitTime":   fmt.Sprintf("%d分钟", transitMinutes),
			})

			lastEndTime = endTime
			totalMinutes = endTime
		}

		// 检测时间冲突
		conflicts := []string{}
		if totalMinutes > 1320 { // 22:00
			conflicts = append(conflicts, "行程结束时间超过22:00，建议缩短游览时间或减少景点")
		}

		totalHours := totalMinutes / 60
		totalMins := totalMinutes % 60

		return map[string]interface{}{
			"status":      "success",
			"totalTime":   fmt.Sprintf("%d小时%d分钟", totalHours, totalMins),
			"startTime":   "09:00",
			"endTime":     fmt.Sprintf("%02d:%02d", totalMinutes/60, totalMinutes%60),
			"hasConflict": len(conflicts) > 0,
			"conflicts":   conflicts,
			"itinerary":   itinerary,
		}, nil
	}

	// 创建时间估算工具
	tool, _ := aitool.New("TimeEstimateAPI",
		aitool.WithDescription("行程时间估算工具 - 估算多地点行程所需时间"),
		aitool.WithSimpleCallback(callback),
		aitool.WithStringArrayParamEx("locations",
			[]aitool.PropertyOption{
				aitool.WithParam_Description("地点列表"),
				aitool.WithParam_Required(),
			},
			aitool.WithParam_Description("地点名称"),
			aitool.WithParam_Required(),
		),
		aitool.WithStringArrayParamEx("durations",
			[]aitool.PropertyOption{
				aitool.WithParam_Description("各地点停留时间"),
				aitool.WithParam_Required(),
			},
			aitool.WithParam_Description("停留时间"),
			aitool.WithParam_Required(),
		),
	)

	return tool
}

func ErrorTool() *aitool.Tool {
	callback := func(params aitool.InvokeParams, stdout io.Writer, stderr io.Writer) (interface{}, error) {
		return nil, fmt.Errorf("这是一个模拟错误的工具")
	}

	tool, _ := aitool.New("error",
		aitool.WithDescription("模拟错误的工具"),
		aitool.WithSimpleCallback(callback),
	)

	return tool
}

// EchoTool 直接返回输入的工具
func EchoTool() *aitool.Tool {
	callback := func(params aitool.InvokeParams, stdout io.Writer, stderr io.Writer) (interface{}, error) {
		input := params.GetString("input")
		return input, nil
	}

	tool, _ := aitool.New("echo",
		aitool.WithDescription("输出测试工具"),
		aitool.WithSimpleCallback(callback),
		aitool.WithStringParam("input",
			aitool.WithParam_Description("直接返回的结果"),
		),
	)

	return tool
}

// PrintTool 直接输出到标准输出的工具
func PrintTool() *aitool.Tool {
	callback := func(params aitool.InvokeParams, stdout io.Writer, stderr io.Writer) (interface{}, error) {
		output := params.GetString("output")
		errString := params.GetString("err")
		if output != "" {
			stdout.Write([]byte(output))
		}

		if errString != "" {
			stderr.Write([]byte(errString))
		}

		return nil, nil
	}

	// 创建交通工具
	tool, _ := aitool.New("print",
		aitool.WithDescription("输出测试工具"),
		aitool.WithSimpleCallback(callback),
		aitool.WithStringParam("output",
			aitool.WithParam_Description("输出"),
		),
		aitool.WithStringParam("err",
			aitool.WithParam_Description("错误输出"),
		),
	)

	return tool
}

// TimeDelayedTool 模拟延时工具
func TimeDelayTool() *aitool.Tool {
	callback := func(params aitool.InvokeParams, stdout io.Writer, stderr io.Writer) (interface{}, error) {
		delay := params.GetInt("delay")
		time.Sleep(time.Duration(delay) * time.Second)
		return nil, nil
	}

	// 创建交通工具
	tool, _ := aitool.New("delay",
		aitool.WithDescription("延时测试工具"),
		aitool.WithSimpleCallback(callback),
		aitool.WithIntegerParam("delay",
			aitool.WithParam_Description("延时时间（秒）"),
		),
	)

	return tool
}
