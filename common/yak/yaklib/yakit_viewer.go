package yaklib

import (
	"fmt"
	uuid "github.com/google/uuid"
	"github.com/yaklang/yaklang/common/utils"
)

func init() {
	YakitExports["EnableWebsiteTrees"] = yakitEnableCrawlerViewer
	YakitExports["EnableTable"] = yakitEnableFixedTable
	YakitExports["EnableText"] = yakitEnableText
	YakitExports["TableData"] = yakitTableData
	YakitExports["StatusCard"] = yakitStatusCard
	YakitExports["TextTabData"] = yakitTextTabData
}

type YakitFeature struct {
	Feature string                 `json:"feature"`
	Params  map[string]interface{} `json:"params"`
}

func yakitEnableCrawlerViewer(targets string) {
	yakitClientInstance.Output(&YakitFeature{
		Feature: "website-trees",
		Params: map[string]interface{}{
			"targets":          targets,
			"refresh_interval": 3,
		},
	})
}

func yakitEnableFixedTable(tableName string, columns []string) {
	yakitClientInstance.Output(&YakitFeature{
		Feature: "fixed-table",
		Params: map[string]interface{}{
			"table_name": tableName,
			"columns":    columns,
		},
	})
}

func yakitEnableText(tabName string) {
	yakitClientInstance.Output(&YakitFeature{
		Feature: "text",
		Params: map[string]interface{}{
			"tab_name": tabName,
		},
	})
}

type YakitFixedTableData struct {
	TableName string                 `json:"table_name"`
	Data      map[string]interface{} `json:"data"`
}

func yakitTableData(tableName string, data any) *YakitFixedTableData {
	tableData := &YakitFixedTableData{
		TableName: tableName,
		Data:      utils.InterfaceToGeneralMap(data),
	}
	if tableData.Data == nil {
		tableData.Data = map[string]interface{}{}
	}
	tableData.Data["uuid"] = uuid.New().String()
	if yakitClientInstance != nil {
		yakitClientInstance.Output(tableData)
	}
	return nil
}

type YakitTextTabData struct {
	TableName string `json:"table_name"`
	Data      string `json:"data"`
}

func yakitTextTabData(tabName string, data string) {
	tabData := &YakitTextTabData{
		TableName: tabName,
		Data:      data,
	}
	if yakitClientInstance != nil {
		yakitClientInstance.Output(tabData)
	}
}

type YakitStatusCard struct {
	Id   string   `json:"id"`
	Data string   `json:"data"`
	Tags []string `json:"tags"`
}

func yakitStatusCard(id string, data interface{}, tags ...string) {
	yakitClientInstance.Output(&YakitStatusCard{
		Id: id, Data: fmt.Sprint(data), Tags: tags,
	})
}
