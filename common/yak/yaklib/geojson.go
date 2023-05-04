package yaklib

import geojson "github.com/paulmach/go.geojson"

var GeoJsonExports = map[string]interface{}{
	"NewFeatureCollection": geojson.NewFeatureCollection,
	"FeaturesToCollection": func(fs ...*geojson.Feature) *geojson.FeatureCollection {
		col := geojson.NewFeatureCollection()
		for _, i := range fs {
			col.AddFeature(i)
		}
		return col
	},
	"WithValue": func(f *geojson.Feature, value float64) *geojson.Feature {
		f.SetProperty("value", value)
		return f
	},
	"WithName": func(f *geojson.Feature, value string) *geojson.Feature {
		f.SetProperty("name", value)
		return f
	},
	"WithNameValue": func(f *geojson.Feature, name string, value float64) *geojson.Feature {
		f.SetProperty("name", name)
		f.SetProperty("value", value)
		return f
	},
	"WithProperty": func(f *geojson.Feature, key string, value float64) *geojson.Feature {
		f.SetProperty(key, value)
		return f
	},
}
