package databasex_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/jinzhu/gorm"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/databasex"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
)

func TestGorm(t *testing.T) {

	db := ssadb.GetDB()

	progName := uuid.NewString()
	defer ssadb.DeleteProgram(db, progName)

	fetch := databasex.NewFetch(func() []*ssadb.IrCode {
		log.Errorf("Fetch IrCode ")
		defer log.Errorf("Fetch IrCode end")
		var irCodes *ssadb.IrCode
		utils.GormTransaction(db, func(tx *gorm.DB) error {
			log.Errorf("Fetch IrCode transaction")
			defer log.Errorf("Fetch IrCode transaction end")
			var id int64
			id, irCodes = ssadb.RequireIrCode(tx, progName)
			_ = id
			return nil
		})
		return []*ssadb.IrCode{irCodes}
	})
	saver := databasex.NewSaver(func(t []string) {
		log.Errorf("Save IrCode ")
		defer log.Errorf("Save IrCode end")

		// Create indices first, then save them after the transaction is complete
		// var indicesToSave []*ssadb.IrIndex

		utils.GormTransaction(db, func(tx *gorm.DB) error {
			log.Errorf("Save IrIndex transaction: %v", t)
			defer log.Errorf("Save IrIndex transaction end")
			for i, item := range t {
				log.Errorf("Create IrIndex item: %s, index: %d", item, i)
				index := ssadb.CreateIndex(tx)
				log.Errorf("Create IrIndex item: %s, index: %d done", item, i)
				index.VariableName = item
				// indicesToSave = append(indicesToSave, index)
				log.Errorf("Save IrIndex item: %s, index: %d", item, i)
				ssadb.SaveIrIndex(tx, index)
			}
			return nil
		})
	})
	log.Errorf("fetch ")
	id, err := fetch.Fetch()
	require.NoError(t, err)
	_ = id
	log.Errorf("save")
	saver.Save("a")

	log.Errorf("send save finish")

	saver.Close()
	fetch.Close()

}
