package amap

import (
	"testing"

	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"gotest.tools/v3/assert"
)

func init() {
	yakit.LoadGlobalNetworkConfig()
}

func TestGeocode(t *testing.T) {
	res, err := Geocode("成都")
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, res[0].Country, "中国")
}

func TestReverseGeocode(t *testing.T) {
	res, err := ReverseGeocode(116.481488, 39.990464)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, res.FormattedAddress, "北京市朝阳区望京街道方恒国际中心B座")
}

func TestGetDrivingPlan(t *testing.T) {
	res, err := DrivingPlan("成都天府三街", "成都天府大道")
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, len(res.Route.Paths) > 0, true)
}

func TestGetWalkingPlan(t *testing.T) {
	res, err := WalkingPlan("成都武侯区", "成都金牛区")
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, len(res.Route.Paths) > 0, true)
}

func TestGetTransitPlan(t *testing.T) {
	res, err := TransitPlan("成都天府三街", "成都天府大道")
	if err != nil {
		t.Fatal(err)
	}
	// 至少有3种出行方案
	assert.Equal(t, len(res.Route.Transits) > 3, true)
}

func TestGetBicyclingPlan(t *testing.T) {
	res, err := BicyclingPlan("成都天府三街", "成都天府大道")
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, len(res.Paths) > 0, true)
}

func TestGetDistance(t *testing.T) {
	res, err := Distance("成都天府三街", "成都天府大道")
	if err != nil {
		t.Fatal(err)
	}
	// 距离大于0
	assert.Equal(t, len(res.Distance) > 0, true)
}

func TestGetIPLocation(t *testing.T) {
	res, err := IPLocation("106.11.25.155")
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, res.Province != "", true)
}

func TestGetWeather(t *testing.T) {
	res, err := GetWeather("成都")
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, len(res.Lives) > 0, true)
}

func TestSearchPOI(t *testing.T) {
	res, err := SearchPOI("成都", WithCity("成都"))
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, len(res.Results) > 0, true)
}

func TestSearchNearbyPOI(t *testing.T) {
	res, err := SearchNearbyPOI("成都", "美食")
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, len(res.Results) > 0, true)
}

func TestGetPOIDetail(t *testing.T) {
	res, err := GetPOIDetail("B0FFHDRT1J")
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, res.Name != "", true)
}
