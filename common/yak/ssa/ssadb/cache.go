package ssadb

import (
	"context"
	"fmt"
	"sync"
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

	// ProgramCacheCapacity = 20 // 20 programs
	ProgramCacheTTL = 10 * time.Minute // 30 minute

	irRelationCount = 5
)

var irTypeSingleFlight singleflight.Group
var initIrTypeOnce = sync.Once{}
var irTypeCaches *utils.CacheEx[*utils.CacheExWithKey[int64, *IrType]]

var irCodeSingleFlight singleflight.Group
var initIrCodeOnce = sync.Once{}
var irCodeCaches *utils.CacheEx[*utils.CacheExWithKey[int64, *IrCode]]

func GetIrTypeCache(progName string) *utils.CacheExWithKey[int64, *IrType] {
	initIrTypeOnce.Do(func() {
		irTypeCaches = utils.NewCacheEx[*utils.CacheExWithKey[int64, *IrType]](
			utils.WithCacheTTL(ProgramCacheTTL),
		)
	})
	if ret, err := irTypeCaches.GetOrLoad(progName, func() (*utils.CacheExWithKey[int64, *IrType], error) {
		return utils.NewCacheExWithKey[int64, *IrType](
			utils.WithCacheCapacity(irCacheCapacity),
			utils.WithCacheTTL(irCacheTTL),
		), nil
	}); err == nil {
		return ret
	} else {
		return nil
	}
}

func deleteCache(progName string) {
	if irCodeCaches != nil {
		irCodeCaches.Delete(progName)
	}
	if irTypeCaches != nil {
		irTypeCaches.Delete(progName)
	}
}

func GetIrCodeCache(progName string) *utils.CacheExWithKey[int64, *IrCode] {
	initIrCodeOnce.Do(func() {
		irCodeCaches = utils.NewCacheEx[*utils.CacheExWithKey[int64, *IrCode]](utils.WithCacheTTL(ProgramCacheTTL))
	})

	if ret, err := irCodeCaches.GetOrLoad(progName, func() (*utils.CacheExWithKey[int64, *IrCode], error) {
		return utils.NewCacheExWithKey[int64, *IrCode](
			utils.WithCacheCapacity(irCacheCapacity),
			utils.WithCacheTTL(irCacheTTL),
		), nil
	}); err == nil {
		return ret
	}
	return nil
}

func dbKey(programName string, id int64) string {
	return fmt.Sprintf("%s_%d", programName, id)
}

func GetIrCodeById(db *gorm.DB, progName string, id int64) *IrCode {
	if id == -1 {
		return nil
	}

	cache := GetIrCodeCache(progName)

	// 先检查缓存
	if ir, ok := cache.Get(id); ok {
		return ir
	}

	// 使用singleflight确保同时只有一个协程查询相同的key
	key := dbKey(progName, id)
	result, _, _ := irCodeSingleFlight.Do(key, func() (interface{}, error) {
		// 再次检查缓存，防止在等待期间其他协程已经加载了数据
		if ir, ok := cache.Get(id); ok {
			return ir, nil
		}

		ctx := context.Background()
		itemsToCache := make(map[int64]*IrCode)
		var ret *IrCode

		utils.GormTransaction(db, func(tx *gorm.DB) error {
			idsToLoad := make(map[int64]struct{})
			addID := func(id int64) {
				// check
				if id < 0 {
					return
				}
				if _, ok := cache.Get(id); ok {
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
			itemsToCache[ret.CodeID] = ret

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
				itemsToCache[ir.CodeID] = ir
			}
			return nil
		})

		// 更新缓存
		for id, item := range itemsToCache {
			cache.Set(id, item)
		}

		return ret, nil
	})

	if result == nil {
		return nil
	}
	return result.(*IrCode)
}

func GetIrTypeById(db *gorm.DB, progName string, id int64) *IrType {
	if id < 0 {
		return nil
	}

	cache := GetIrTypeCache(progName)

	// 先检查缓存
	if ir, ok := cache.Get(id); ok {
		return ir
	}

	// 使用singleflight确保同时只有一个协程查询相同的key
	key := dbKey(progName, id)
	result, _, _ := irTypeSingleFlight.Do(key, func() (interface{}, error) {
		// 再次检查缓存，防止在等待期间其他协程已经加载了数据
		if ir, ok := cache.Get(id); ok {
			return ir, nil
		}

		ctx := context.Background()
		itemsToCache := make(map[int64]*IrType)
		var ret *IrType

		utils.GormTransaction(db, func(tx *gorm.DB) error {
			idsToLoad := make(map[int64]struct{})
			addID := func(id int64) {
				// check
				if id < 0 {
					return
				}
				if _, ok := cache.Get(id); ok {
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
			itemsToCache[int64(ret.TypeId)] = ret

			// load [id-relation ... id ... id+relation]
			for i := int64(1); i < irRelationCount; i++ {
				addID(id + i)
				addID(id - i)
			}

			// update cache
			ids := lo.MapToSlice(idsToLoad, func(key int64, _ struct{}) int64 { return key })
			tx = bizhelper.ExactQueryInt64ArrayOr(tx, "type_id", ids).Where("program_name = ?", progName)
			for ir := range bizhelper.YieldModel[*IrType](ctx, tx, bizhelper.WithYieldModel_Fast()) {
				itemsToCache[int64(ir.TypeId)] = ir
			}
			return nil
		})

		// 更新缓存
		for id, item := range itemsToCache {
			cache.Set(id, item)
		}

		return ret, nil
	})

	if result == nil {
		return nil
	}
	return result.(*IrType)
}
