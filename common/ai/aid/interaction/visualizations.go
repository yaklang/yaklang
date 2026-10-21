package interaction

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
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
		if (props["selection"] == nil || props["selection"] == "none") && node.Action != nil {
			node.Action = nil
			diagnose("action", "selection is disabled")
		}
	}
	return true
}

// NormalizeSubmission derives table receipts from the server's document. A client
// may select row IDs, but cannot supply authoritative row values or extra fields.
func (doc ComponentDocument) NormalizeSubmission(receipt Submission) (Submission, error) {
	for _, node := range doc.Nodes {
		if node.ID != receipt.NodeID || node.Component != "DataTable" {
			continue
		}
		selection, _ := node.Props["selection"].(string)
		if selection != "single" && selection != "multiple" {
			return receipt, fmt.Errorf("table selection is disabled")
		}
		for key := range receipt.Values {
			if key != "selectedRowIds" && key != "rows" {
				return receipt, fmt.Errorf("undeclared table submission field")
			}
		}
		ids, ok := receipt.Values["selectedRowIds"].([]any)
		if !ok || len(ids) == 0 || len(ids) > 200 || (selection == "single" && len(ids) != 1) {
			return receipt, fmt.Errorf("invalid table selection")
		}
		selected := map[string]bool{}
		for _, value := range ids {
			id, ok := value.(string)
			if !ok || id == "" || selected[id] {
				return receipt, fmt.Errorf("missing or duplicate selected row id")
			}
			selected[id] = true
		}
		rows, ok := uiObjects(node.Props["rows"])
		if !ok {
			return receipt, fmt.Errorf("invalid table rows")
		}
		acceptedIDs, acceptedRows := []any{}, []any{}
		for _, row := range rows {
			id, _ := row["id"].(string)
			if selected[id] {
				acceptedIDs = append(acceptedIDs, id)
				acceptedRows = append(acceptedRows, map[string]any{"id": id, "values": row["values"]})
			}
		}
		if len(acceptedIDs) != len(ids) {
			return receipt, fmt.Errorf("selected row does not belong to this table")
		}
		receipt.Values = map[string]any{"selectedRowIds": acceptedIDs, "rows": acceptedRows}
		return receipt, nil
	}
	return receipt, nil
}

func tableSubmissionText(node ComponentNode, receipt Submission) (string, error) {
	rows, ok := uiObjects(receipt.Values["rows"])
	if !ok {
		return "", fmt.Errorf("missing canonical table rows")
	}
	columns, ok := uiObjects(node.Props["columns"])
	if !ok {
		return "", fmt.Errorf("invalid table columns")
	}
	title, _ := node.Props["title"].(string)
	lines := []string{"数据表选择：" + title}
	for _, row := range rows {
		values, _ := row["values"].(map[string]any)
		cells := []string{}
		for _, column := range columns {
			key, _ := column["key"].(string)
			label, _ := column["label"].(string)
			value := "—"
			if values[key] != nil {
				value = fmt.Sprint(values[key])
			}
			cells = append(cells, label+"："+value)
		}
		lines = append(lines, "- "+fmt.Sprint(row["id"])+"："+strings.Join(cells, "；"))
	}
	return strings.Join(lines, "\n"), nil
}

// DisplayOnly retains local inspection while removing message-producing actions.
func (entry ComponentDefinition) DisplayOnly() ComponentDefinition {
	raw, _ := json.Marshal(entry)
	var copy ComponentDefinition
	_ = json.Unmarshal(raw, &copy)
	copy.Actions, copy.Bindings = nil, nil
	if props, ok := copy.Schema["properties"].(map[string]any); ok {
		props["selection"] = map[string]any{"type": "string", "enum": []any{"none"}, "default": "none"}
	}
	copy.Description = "只读数据明细。允许本地排序、筛选、分页；当前会话禁止行选择、状态绑定和提交动作。"
	return copy
}
