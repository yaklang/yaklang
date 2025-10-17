package utils

import (
	"fmt"
	"sync"

	"github.com/jinzhu/gorm"
)

func GormTransaction(db *gorm.DB, callback func(tx *gorm.DB) error) (err error) {
	tx := db.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			panic(r)
		} else if err != nil {
			tx.Rollback()
		} else {
			db := tx.Commit()
			if db != nil {
				err = db.Error
			}
		}
	}()

	err = callback(tx)
	return
}

func GormTransactionReturnDb(db *gorm.DB, callback func(tx *gorm.DB)) (tx *gorm.DB) {
	tx = db.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			panic(r)
		} else {
			if tx.Error != nil {
				tx.Rollback()
			} else {
				tx.Commit()
			}
		}
	}()
	callback(tx)
	return
}

// SavepointTransaction 基于SAVEPOINT的多层事务管理器
type SavepointTransaction struct {
	db               *gorm.DB
	savepointStack   []string
	isInTransaction  bool
	mutex            sync.RWMutex
	savepointCounter int
}

// NewSavepointTransaction 创建一个新的SAVEPOINT事务管理器
func NewSavepointTransaction(db *gorm.DB) *SavepointTransaction {
	return &SavepointTransaction{
		db:              db,
		savepointStack:  make([]string, 0),
		isInTransaction: false,
		mutex:           sync.RWMutex{},
	}
}

// Begin 开始一个新的事务层级
func (st *SavepointTransaction) Begin() (*gorm.DB, error) {
	st.mutex.Lock()
	defer st.mutex.Unlock()

	if !st.isInTransaction {
		// 第一层：开始真正的事务
		tx := st.db.Begin()
		if tx.Error != nil {
			return nil, tx.Error
		}
		st.db = tx
		st.isInTransaction = true
		st.savepointStack = append(st.savepointStack, "MAIN_TRANSACTION")
		return st.db, nil
	} else {
		// 嵌套层：创建SAVEPOINT
		st.savepointCounter++
		savepointName := fmt.Sprintf("sp_%d", st.savepointCounter)

		err := st.db.Exec(fmt.Sprintf("SAVEPOINT %s", savepointName)).Error
		if err != nil {
			return nil, err
		}

		st.savepointStack = append(st.savepointStack, savepointName)
		return st.db, nil
	}
}

// Commit 提交当前层级的事务
func (st *SavepointTransaction) Commit() error {
	st.mutex.Lock()
	defer st.mutex.Unlock()

	if len(st.savepointStack) == 0 {
		return fmt.Errorf("no active transaction to commit")
	}

	// 弹出最后一个savepoint
	lastSavepoint := st.savepointStack[len(st.savepointStack)-1]
	st.savepointStack = st.savepointStack[:len(st.savepointStack)-1]

	if lastSavepoint == "MAIN_TRANSACTION" {
		// 最外层事务：真正提交
		err := st.db.Commit().Error
		st.isInTransaction = false
		return err
	} else {
		// 嵌套事务：释放SAVEPOINT
		return st.db.Exec(fmt.Sprintf("RELEASE SAVEPOINT %s", lastSavepoint)).Error
	}
}

// Rollback 回滚当前层级的事务
func (st *SavepointTransaction) Rollback() error {
	st.mutex.Lock()
	defer st.mutex.Unlock()

	if len(st.savepointStack) == 0 {
		return fmt.Errorf("no active transaction to rollback")
	}

	// 弹出最后一个savepoint
	lastSavepoint := st.savepointStack[len(st.savepointStack)-1]
	st.savepointStack = st.savepointStack[:len(st.savepointStack)-1]

	if lastSavepoint == "MAIN_TRANSACTION" {
		// 最外层事务：真正回滚
		err := st.db.Rollback().Error
		st.isInTransaction = false
		return err
	} else {
		// 嵌套事务：回滚到SAVEPOINT
		return st.db.Exec(fmt.Sprintf("ROLLBACK TO SAVEPOINT %s", lastSavepoint)).Error
	}
}

// GetDB 获取当前的数据库连接
func (st *SavepointTransaction) GetDB() *gorm.DB {
	st.mutex.RLock()
	defer st.mutex.RUnlock()
	return st.db
}

// IsInTransaction 检查是否在事务中
func (st *SavepointTransaction) IsInTransaction() bool {
	st.mutex.RLock()
	defer st.mutex.RUnlock()
	return st.isInTransaction
}

// GetLevel 获取当前事务层级
func (st *SavepointTransaction) GetLevel() int {
	st.mutex.RLock()
	defer st.mutex.RUnlock()
	return len(st.savepointStack)
}

// SavepointTransactionWithCallback 基于SAVEPOINT的事务回调函数
func SavepointTransactionWithCallback(db *gorm.DB, callback func(st *SavepointTransaction) error) error {
	st := NewSavepointTransaction(db)

	_, err := st.Begin()
	if err != nil {
		return err
	}

	defer func() {
		if r := recover(); r != nil {
			st.Rollback()
			panic(r)
		}
	}()

	err = callback(st)
	if err != nil {
		st.Rollback()
		return err
	}

	return st.Commit()
}

// NestedSavepointTransaction 嵌套SAVEPOINT事务，可以在已存在的SavepointTransaction中使用
func NestedSavepointTransaction(st *SavepointTransaction, callback func(*gorm.DB) error) error {
	_, err := st.Begin()
	if err != nil {
		return err
	}

	defer func() {
		if r := recover(); r != nil {
			st.Rollback()
			panic(r)
		}
	}()

	err = callback(st.GetDB())
	if err != nil {
		st.Rollback()
		return err
	}

	return st.Commit()
}
