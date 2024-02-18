package yaklib

import (
	"fmt"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

func Test_ExactQueryInt64ArrayOr(t *testing.T) {
	db := consts.GetGormProjectDatabase()
	db = bizhelper.ExactQueryInt64ArrayOr(db, "id", []int64{1, 2, 3, 6, 7, 8, 9, 10, 15, 17, 18})

	scope := db.NewScope(&yakit.HTTPFlow{})
	sqlCommand := scope.CombinedConditionSql()
	for _, v := range scope.SQLVars {
		sqlCommand = strings.Replace(sqlCommand, "$$$", fmt.Sprintf("%v", v), 1)
	}
	log.Infof("ExactQueryInt64ArrayOr sqlCommand: %v", sqlCommand)
	if !strings.Contains(sqlCommand, "id >= 1 AND id <= 3") || !strings.Contains(sqlCommand, "id >= 6 AND id <= 10") || !strings.Contains(sqlCommand, "id = 15") || !strings.Contains(sqlCommand, "id >= 17 AND id <= 18") {
		t.Fatal("ExactQueryInt64ArrayOr failed")
	}
}

func Test_ExactExcludeQueryInt64Array(t *testing.T) {
	db := consts.GetGormProjectDatabase()
	db = bizhelper.ExactExcludeQueryInt64Array(db, "id", []int64{1, 2, 3, 6, 7, 8, 9, 10, 15, 17, 18})

	scope := db.NewScope(&yakit.HTTPFlow{})
	sqlCommand := scope.CombinedConditionSql()
	for _, v := range scope.SQLVars {
		sqlCommand = strings.Replace(sqlCommand, "$$$", fmt.Sprintf("%v", v), 1)
	}
	log.Infof("ExactExcludeQueryInt64Array sqlCommand: %v", sqlCommand)
	if !strings.Contains(sqlCommand, "id < 1 OR id > 3") || !strings.Contains(sqlCommand, "id < 6 OR id > 10") || !strings.Contains(sqlCommand, "id <> 15") || !strings.Contains(sqlCommand, "id < 17 OR id > 18") {
		t.Fatal("ExactExcludeQueryInt64Array failed")
	}
}
