package consts

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
)

func TestSSAProjectDatabaseConcurrentPublication(t *testing.T) {
	db := GetGormSSAProjectDataBase()
	require.NotNil(t, db)
	var wg sync.WaitGroup
	for worker := 0; worker < 16; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 100; i++ {
				SetGormSSAProjectDatabase(db)
				if GetGormSSAProjectDataBase() != db || schema.GetDefaultSSADatabase() != db {
					t.Error("concurrent SSA database publication lost its handle")
				}
			}
		}()
	}
	wg.Wait()
}
