package main

import (
	"github.com/yaklang/yaklang/common/consts"
)

func init() {
	db := consts.GetGormProfileDatabase()
	autoAutomigrateVectorStoreDocument(db)

	go func() {
		db := consts.GetGormProfileDatabase()
		autoAutomigrateVectorStoreDocument(db)
	}()
}
