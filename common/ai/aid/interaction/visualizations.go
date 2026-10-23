package interaction

import (
	"fmt"
	"math"
)

func uiObjects(value any) ([]map[string]any, bool) {
	values, ok := value.([]any)
	if !ok {
		return nil, false
	}
	result := make([]map[string]any, 0, len(values))
	for _, value := range values {
		item, ok := value.(map[string]any)
		if !ok {
			return nil, false
		}
		result = append(result, item)
	}
	return result, true
}

func uniqueUIField(items []map[string]any, field string) bool {
	seen := map[string]bool{}
	for _, item := range items {
		id, ok := item[field].(string)
		if !ok || id == "" || seen[id] {
			return false
		}
		seen[id] = true
	}
	return true
}

func finiteUIValue(value any) bool {
	number, ok := value.(float64)
	return ok && !math.IsNaN(number) && !math.IsInf(number, 0)
}

// NormalizeVisualization checks relationships that cannot be expressed by the
// portable catalog Schema. It runs after field normalization on both ends.
func NormalizeVisualization(node *ComponentNode, diagnose func(string, string)) bool {
	props := node.Props
	switch node.Component {
	case "BarChart", "LineChart":
		series, ok := uiObjects(props["series"])
		if !ok || len(series) == 0 || !uniqueUIField(series, "id") {
			diagnose("props.series", "missing or duplicate series ids")
			return false
		}
		if node.Component == "BarChart" {
			categories, ok := uiObjects(props["categories"])
			if !ok || len(categories) == 0 || !uniqueUIField(categories, "id") {
				diagnose("props.categories", "missing or duplicate category ids")
				return false
			}
			for _, item := range series {
				values, ok := item["values"].([]any)
				if !ok || len(values) != len(categories) {
					diagnose("props.series", "values must match categories in length and order")
					return false
				}
				for _, value := range values {
					if value != nil && !finiteUIValue(value) {
						diagnose("props.series", "values must be finite numbers or null")
						return false
					}
				}
			}
		} else {
			for _, item := range series {
				points, ok := uiObjects(item["points"])
				if !ok || len(points) == 0 {
					diagnose("props.series", "points are required")
					return false
				}
				previous := math.Inf(-1)
				for _, point := range points {
					x, ok := point["x"].(float64)
					if !ok || !finiteUIValue(x) || x <= previous || (point["y"] != nil && !finiteUIValue(point["y"])) ||
						(props["xType"] == "time" && (math.Trunc(x) != x || math.Abs(x) > 8640000000000000)) {
						diagnose("props.series", "x must be valid and strictly increasing; values must be finite")
						return false
					}
					previous = x
				}
			}
		}
	case "DataTable":
		columns, columnsOK := uiObjects(props["columns"])
		rows, rowsOK := uiObjects(props["rows"])
		if !columnsOK || !rowsOK || len(columns) == 0 || !uniqueUIField(columns, "key") || !uniqueUIField(rows, "id") {
			diagnose("props", "missing or duplicate column keys or row ids")
			return false
		}
		byKey := map[string]map[string]any{}
		for _, column := range columns {
			byKey[column["key"].(string)] = column
		}
		for index, row := range rows {
			values, ok := row["values"].(map[string]any)
			if !ok {
				diagnose("props.rows", "row values must be an object")
				return false
			}
			for key, value := range values {
				path := fmt.Sprintf("props.rows[%d].values.%s", index, key)
				column := byKey[key]
				if column == nil {
					delete(values, key)
					diagnose(path, "undeclared column removed")
					continue
				}
				if value == nil {
					continue
				}
				valid := false
				switch column["type"] {
				case "string":
					_, valid = value.(string)
				case "number":
					valid = finiteUIValue(value)
				case "boolean":
					_, valid = value.(bool)
				}
				if !valid {
					values[key] = nil
					diagnose(path, "cell type mismatch; cleared")
				}
			}
		}
		if initial, ok := props["initialSort"].(map[string]any); ok {
			key, _ := initial["key"].(string)
			if byKey[key] == nil || byKey[key]["sortable"] == false {
				delete(props, "initialSort")
				diagnose("props.initialSort", "unknown or unsortable column")
			}
		}

	}
	return true
}
