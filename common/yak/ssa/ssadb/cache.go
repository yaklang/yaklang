package ssadb

import (
	"context"
	"fmt"
	"time"

	"github.com/jinzhu/gorm"
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"golang.org/x/sync/singleflight"
)

const (
	irCacheCapacity = 100000          // 10w
	irCacheTTL      = 5 * time.Minute // 5 minute
	irRelationCount = 5
)

var irTypeCache = utils.NewCacheEx[*IrType](
	utils.WithCacheCapacity(irCacheCapacity),
	utils.WithCacheTTL(irCacheTTL),
)

var irCodeCache = utils.NewCacheEx[*IrCode](
	utils.WithCacheCapacity(irCacheCapacity),
	utils.WithCacheTTL(irCacheTTL),
)

var irCodeSingleFlight singleflight.Group
var irTypeSingleFlight singleflight.Group

func dbKey(programName string, id int64) string {
	return fmt.Sprintf("%s_%d", programName, id)
}

func GetIrCodeById(db *gorm.DB, progName string, id int64) *IrCode {
	if id == -1 {
		return nil
	}

	key := dbKey(progName, id)

	// 先检查缓存
	if ir, ok := irCodeCache.Get(key); ok {
		return ir
	}

	// 使用singleflight确保同时只有一个协程查询相同的key
	result, _, _ := irCodeSingleFlight.Do(key, func() (interface{}, error) {
		// 再次检查缓存，防止在等待期间其他协程已经加载了数据
		if ir, ok := irCodeCache.Get(key); ok {
			return ir, nil
		}

		ctx := context.Background()
		itemsToCache := make(map[string]*IrCode)
		var ret *IrCode

		utils.GormTransaction(db, func(tx *gorm.DB) error {
			idsToLoad := make(map[int64]struct{})
			addID := func(id int64) {
				// check
				if id < 0 {
					return
				}
				if _, ok := irCodeCache.Get(dbKey(progName, id)); ok {
					return
				}
				if _, ok := idsToLoad[id]; ok {
					return
				}
				idsToLoad[id] = struct{}{}
			}

			// load self
			ret = GetIrCodeItemById(tx, progName, id)
			if ret == nil {
				return nil
			}
			itemsToCache[dbKey(progName, ret.CodeID)] = ret

			// load user and function/block
			for _, userId := range ret.Users {
				addID(userId)
			}
			addID(ret.CurrentFunction)
			addID(ret.CurrentBlock)

			// load [id-relation ... id ... id+relation]
			for i := int64(1); i < irRelationCount; i++ {
				addID(id + i)
				addID(id - i)
			}

			// update cache
			ids := lo.MapToSlice(idsToLoad, func(key int64, _ struct{}) int64 { return key })
			tx = bizhelper.ExactQueryInt64ArrayOr(tx, "code_id", ids).Where("program_name = ?", progName)
			for ir := range bizhelper.YieldModel[*IrCode](ctx, tx, bizhelper.WithYieldModel_Fast()) {
				itemsToCache[dbKey(progName, ir.CodeID)] = ir
			}
			return nil
		})

		// 更新缓存
		for key, item := range itemsToCache {
			irCodeCache.Set(key, item)
		}

		return ret, nil
	})

	if result == nil {
		return nil
	}
	return result.(*IrCode)
}

func GetIrTypeById(db *gorm.DB, progName string, id int64) *IrType {
	if id == -1 {
		return nil
	}

	key := dbKey(progName, id)

	// 先检查缓存
	if ir, ok := irTypeCache.Get(key); ok {
		return ir
	}

	// 使用singleflight确保同时只有一个协程查询相同的key
	result, _, _ := irTypeSingleFlight.Do(key, func() (interface{}, error) {
		// 再次检查缓存，防止在等待期间其他协程已经加载了数据
		if ir, ok := irTypeCache.Get(key); ok {
			return ir, nil
		}

		ctx := context.Background()
		itemsToCache := make(map[string]*IrType)
		var ret *IrType

		utils.GormTransaction(db, func(tx *gorm.DB) error {
			idsToLoad := make(map[int64]struct{})
			addID := func(id int64) {
				// check
				if id < 0 {
					return
				}
				if _, ok := irTypeCache.Get(dbKey(progName, id)); ok {
					return
				}
				if _, ok := idsToLoad[id]; ok {
					return
				}
				idsToLoad[id] = struct{}{}
			}

			// load self
			ret = GetIrTypeItemById(tx, progName, id)
			if ret == nil {
				return nil
			}
			itemsToCache[dbKey(progName, int64(ret.TypeId))] = ret

			// load [id-relation ... id ... id+relation]
			for i := int64(1); i < irRelationCount; i++ {
				addID(id + i)
				addID(id - i)
			}

			// update cache
			ids := lo.MapToSlice(idsToLoad, func(key int64, _ struct{}) int64 { return key })
			tx = bizhelper.ExactQueryInt64ArrayOr(tx, "type_id", ids).Where("program_name = ?", progName)
			for ir := range bizhelper.YieldModel[*IrType](ctx, tx, bizhelper.WithYieldModel_Fast()) {
				itemsToCache[dbKey(progName, int64(ir.TypeId))] = ir
			}
			return nil
		})

		// 更新缓存
		for key, item := range itemsToCache {
			irTypeCache.Set(key, item)
		}

		return ret, nil
	})

	if result == nil {
		return nil
	}
	return result.(*IrType)
}
