package comparer

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"yaklang/common/go-funk"
	"yaklang/common/utils"
)

func getSlice(v reflect.Value, index int) (_ reflect.Value, err error) {
	defer func() {
		err = utils.Error(recover())
	}()
	return v.Index(index), nil
}

func getMapIndex(v reflect.Value, index reflect.Value) (_ reflect.Value, err error) {
	defer func() {
		if perr := recover(); perr != nil {
			err = utils.Error(perr)
		}
	}()
	return v.MapIndex(reflect.ValueOf(index.String())), nil
}

func CompareJsons(s1, s2 []byte) float64 {
	var raw1, raw2 interface{} = nil, nil
	_ = json.Unmarshal(s1, &raw1)
	_ = json.Unmarshal(s2, &raw2)
	if raw1 == nil || raw2 == nil {
		return CompareHtml(s1, s2)
	}
	return compareJsons(raw1, raw2)
}

func compareJsons(raw1, raw2 interface{}) float64 {
	if raw2 == nil && raw1 == nil {
		return 1
	}

	if raw2 == nil || raw1 == nil {
		return 0
	}

	type1, type2 := reflect.TypeOf(raw1), reflect.TypeOf(raw2)
	// 根基类型不对
	if type1.Kind() != type2.Kind() {
		return 0
	}
	v1, v2 := reflect.ValueOf(raw1), reflect.ValueOf(raw2)

	// JSON 一般来说有多种类型
	switch type1.Kind() {
	case reflect.String:
		return compareString(v1.String(), v2.String())
	case reflect.Float64, reflect.Float32, reflect.Bool, reflect.Int64:
		if fmt.Sprint(v1.Interface()) == fmt.Sprint(v2.Interface()) {
			return 1
		} else {
			return 0
		}
	case reflect.Map:
		var list1, list2 []reflect.Value
		vmr1 := v1.MapRange()
		for vmr1.Next() {
			list1 = append(list1, vmr1.Key())
		}
		vmr2 := v2.MapRange()
		for vmr2.Next() {
			list2 = append(list2, vmr2.Key())
		}
		sort.Stable(ReflectValueSortable(list1))
		sort.Stable(ReflectValueSortable(list2))
		var results []float64
		for _, e1 := range append(list1, list2...) {
			var a1, a2 interface{}
			fv1, err := getMapIndex(v1, e1)
			if err == nil && fv1.IsValid() {
				a1 = fv1.Interface()
			}

			fv2, err := getMapIndex(v2, e1)
			if err == nil && fv2.IsValid() {
				a2 = fv2.Interface()
			}
			results = append(results, compareJsons(a1, a2))
		}

		if len(results) > 0 {
			return funk.SumFloat64(results) / float64(len(results))
		}
		return 1
	case reflect.Array, reflect.Slice:
		len1, len2 := v1.Len(), v2.Len()
		var list1, list2 []interface{}
		for i := 0; i < len1; i++ {
			list1 = append(list1, v1.Index(i).Interface())
		}
		for i := 0; i < len2; i++ {
			list2 = append(list2, v2.Index(i).Interface())
		}
		sort.Stable(Sortable(list1))
		sort.Stable(Sortable(list2))

		var maxLength = len1
		if len2 > len1 {
			maxLength = len2
		}
		if maxLength > 0 {
			var results []float64
			for i := 0; i < maxLength; i++ {
				var e1, e2 interface{}
				if i < len(list1) {
					e1 = list1[i]
				}
				if i < len(list2) {
					e2 = list2[i]
				}
				results = append(results, compareJsons(e1, e2))
			}
			return funk.SumFloat64(results) / float64(len(results))
		}
		return 1
	default:
		return compareString(fmt.Sprintf("%#v", raw1), fmt.Sprintf("%#v", raw2))
	}
}
