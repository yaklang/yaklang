// Package mitmextractdb holds MITM rule extract persistence queries (GORM/SQL), separate from yakgrpc/yakit helpers.
package mitmextractdb

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/schema"
)

// JoinExtractedDataWithHTTPFlow builds inner join extracted_data AS ed to http_flows AS hf on trace_id = hidden_index.
func JoinExtractedDataWithHTTPFlow(db *gorm.DB) *gorm.DB {
	ed := db.NewScope(&schema.ExtractedData{}).TableName()
	hf := db.NewScope(&schema.HTTPFlow{}).TableName()
	return db.Table(ed + " AS ed").
		Joins("INNER JOIN " + hf + " AS hf ON ed.trace_id = hf.hidden_index").
		Where("ed.trace_id != ?", "").
		Where("hf.hidden_index != ?", "")
}
