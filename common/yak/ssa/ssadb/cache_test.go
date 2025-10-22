package ssadb

import (
	"testing"

	"github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/sqlite"
	"github.com/stretchr/testify/assert"
)

func setupTestDB() *gorm.DB {
	// 使用临时内存数据库进行测试
	db, err := gorm.Open("sqlite3", ":memory:")
	if err != nil {
		panic("failed to connect database")
	}

	// 自动迁移表结构
	db.AutoMigrate(&IrCode{}, &IrType{}, &IrProgram{})
	return db
}

func TestGetIrCodeCache(t *testing.T) {
	progName := "test_program"

	// 测试获取缓存
	cache1 := GetIrCodeCache(progName)
	assert.NotNil(t, cache1)

	// 测试再次获取相同程序名的缓存应该返回同一个实例
	cache2 := GetIrCodeCache(progName)
	assert.Equal(t, cache1, cache2)

	// 测试不同程序名应该返回不同的缓存实例
	cache3 := GetIrCodeCache("another_program")
	assert.NotEqual(t, cache1, cache3)
}

func TestGetIrTypeCache(t *testing.T) {
	progName := "test_program"

	// 测试获取缓存
	cache1 := GetIrTypeCache(progName)
	assert.NotNil(t, cache1)

	// 测试再次获取相同程序名的缓存应该返回同一个实例
	cache2 := GetIrTypeCache(progName)
	assert.Equal(t, cache1, cache2)

	// 测试不同程序名应该返回不同的缓存实例
	cache3 := GetIrTypeCache("another_program")
	assert.NotEqual(t, cache1, cache3)
}

func TestGetIrCodeById(t *testing.T) {
	db := setupTestDB()
	defer db.Close()

	progName := "test_program"

	// 创建测试数据
	testCode := &IrCode{
		ProgramName: progName,
		CodeID:      1,
		OpcodeName:  "test_op",
		String:      "test_value",
		Name:        "test_name",
	}

	err := db.Create(testCode).Error
	assert.NoError(t, err)

	// 测试从数据库获取并缓存
	result := GetIrCodeById(db, progName, 1)
	assert.NotNil(t, result)
	assert.Equal(t, int64(1), result.CodeID)
	assert.Equal(t, progName, result.ProgramName)
	assert.Equal(t, "test_op", result.OpcodeName)

	// 验证缓存是否生效
	cache := GetIrCodeCache(progName)
	cachedResult, exists := cache.Get(1)
	assert.True(t, exists)
	assert.Equal(t, result, cachedResult)

	// 测试获取不存在的记录
	result2 := GetIrCodeById(db, progName, 999)
	assert.Nil(t, result2)

	// 测试无效ID
	result3 := GetIrCodeById(db, progName, -1)
	assert.Nil(t, result3)
}

func TestGetIrTypeById(t *testing.T) {
	db := setupTestDB()
	defer db.Close()

	progName := "test_program"

	// 创建测试数据
	testType := &IrType{
		ProgramName: progName,
		TypeId:      1,
		Kind:        1,
		String:      "test_string",
	}

	err := db.Create(testType).Error
	assert.NoError(t, err)

	// 测试从数据库获取并缓存
	result := GetIrTypeById(db, progName, 1)
	assert.NotNil(t, result)
	assert.Equal(t, uint64(1), result.TypeId)
	assert.Equal(t, progName, result.ProgramName)
	assert.Equal(t, 1, result.Kind)

	// 验证缓存是否生效
	cache := GetIrTypeCache(progName)
	cachedResult, exists := cache.Get(1)
	assert.True(t, exists)
	assert.Equal(t, result, cachedResult)

	// 测试获取不存在的记录
	result2 := GetIrTypeById(db, progName, 999)
	assert.Nil(t, result2)

	// 测试无效ID
	result3 := GetIrTypeById(db, progName, -1)
	assert.Nil(t, result3)
}

func TestCacheIsolation(t *testing.T) {
	db := setupTestDB()
	defer db.Close()

	progName1 := "program1"
	progName2 := "program2"

	// 为两个不同的程序创建相同ID的测试数据
	testCode1 := &IrCode{
		ProgramName: progName1,
		CodeID:      1,
		OpcodeName:  "op1",
		String:      "value1",
	}
	testCode2 := &IrCode{
		ProgramName: progName2,
		CodeID:      1,
		OpcodeName:  "op2",
		String:      "value2",
	}

	err := db.Create(testCode1).Error
	assert.NoError(t, err)
	err = db.Create(testCode2).Error
	assert.NoError(t, err)

	// 获取两个程序的数据
	result1 := GetIrCodeById(db, progName1, 1)
	result2 := GetIrCodeById(db, progName2, 1)

	// 验证数据正确性和缓存隔离
	assert.NotNil(t, result1)
	assert.NotNil(t, result2)
	assert.Equal(t, "op1", result1.OpcodeName)
	assert.Equal(t, "op2", result2.OpcodeName)
	assert.NotEqual(t, result1, result2)

	// 验证缓存隔离
	cache1 := GetIrCodeCache(progName1)
	cache2 := GetIrCodeCache(progName2)
	assert.NotEqual(t, cache1, cache2)

	cached1, exists1 := cache1.Get(1)
	cached2, exists2 := cache2.Get(1)
	assert.True(t, exists1)
	assert.True(t, exists2)
	assert.Equal(t, result1, cached1)
	assert.Equal(t, result2, cached2)
}

func TestDbKeyFunction(t *testing.T) {
	progName := "test_program"
	id := int64(123)

	key := dbKey(progName, id)
	expected := "test_program_123"
	assert.Equal(t, expected, key)
}

func TestCacheConcurrency(t *testing.T) {
	db := setupTestDB()
	defer db.Close()

	progName := "test_program"

	// 创建测试数据
	testCode := &IrCode{
		ProgramName: progName,
		CodeID:      1,
		OpcodeName:  "test_op",
		String:      "test_value",
	}

	err := db.Create(testCode).Error
	assert.NoError(t, err)

	// 并发测试
	const numGoroutines = 10
	results := make([]*IrCode, numGoroutines)
	done := make(chan bool, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(index int) {
			results[index] = GetIrCodeById(db, progName, 1)
			done <- true
		}(i)
	}

	// 等待所有goroutine完成
	for i := 0; i < numGoroutines; i++ {
		<-done
	}

	// 验证所有结果都相同且正确
	for i := 0; i < numGoroutines; i++ {
		assert.NotNil(t, results[i])
		assert.Equal(t, int64(1), results[i].CodeID)
		assert.Equal(t, progName, results[i].ProgramName)
		if i > 0 {
			assert.Equal(t, results[0], results[i])
		}
	}
}
