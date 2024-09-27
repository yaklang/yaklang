package vulinbox

import (
	"fmt"
	"github.com/jinzhu/gorm"
	_ "github.com/mattn/go-sqlite3"
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

type dbm struct {
	db *gorm.DB
}

func newDBM() (*dbm, error) {
	fp, err := consts.TempFile("*.db")
	if err != nil {
		return nil, err
	}
	name := fp.Name()
	log.Infof("db file: %s", name)
	fp.Close()
	db, err := consts.CreateVulinboxDatabase(name)
	if err != nil {
		return nil, err
	}

	var i any
	if err := db.Raw("select md5('a')").Row().Scan(&i); err != nil {
		return nil, utils.Errorf("sqlite3 md5 function register failed: %v", err)
	}
	if fmt.Sprint(i) != codec.Md5("a") {
		return nil, utils.Errorf("sqlite3 md5 function register failed: %v", i)
	}
	log.Infof("verify md5 function is called: %v", i)

	db.Save(&Session{
		Uuid:     "fixedSessionID",
		Username: "admin",
		Role:     "admin",
	})

	db.Save(&UserOrder{
		UserID:         1,
		ProductName:    "商品1",
		Quantity:       1,
		TotalPrice:     89,
		DeliveryStatus: "已发货",
	})

	db.Save(&UserOrder{
		UserID:         2,
		ProductName:    "商品2",
		Quantity:       2,
		TotalPrice:     178,
		DeliveryStatus: "已发货",
	})
	db.Save(&UserOrder{
		UserID:         2,
		ProductName:    "商品1",
		Quantity:       3,
		TotalPrice:     267,
		DeliveryStatus: "已发货",
	})
	db.Save(&UserOrder{
		UserID:         3,
		ProductName:    "商品4",
		Quantity:       5,
		TotalPrice:     445,
		DeliveryStatus: "已发货",
	})
	db.Save(&UserOrder{
		UserID:         4,
		ProductName:    "商品3",
		Quantity:       1,
		TotalPrice:     89,
		DeliveryStatus: "已发货",
	})

	db.Save(&UserCart{
		UserID:          1,
		ProductName:     "商品2",
		Description:     "这是商品2的描述信息。",
		ProductPrice:    89,
		ProductQuantity: 2,
	})
	db.Save(&UserCart{
		UserID:          1,
		ProductName:     "商品1",
		Description:     "这是商品1的描述信息。",
		ProductPrice:    89,
		ProductQuantity: 3,
	})
	db.Save(&UserCart{
		UserID:          2,
		ProductName:     "商品5",
		Description:     "这是商品5的描述信息。",
		ProductPrice:    89,
		ProductQuantity: 3,
	})
	db.Save(&UserCart{
		UserID:          3,
		ProductName:     "商品4",
		Description:     "这是商品4的描述信息。",
		ProductPrice:    89,
		ProductQuantity: 5,
	})
	db.Save(&VulinUser{
		Username: "admin",
		Password: "admin",
		Age:      25,
		Role:     "admin",
		Remake:   "我是管理员",
	})
	db.Save(&VulinUser{
		Username: "root",
		Password: "p@ssword",
		Age:      25,
		Role:     "admin",
		Remake:   "我是管理员",
	})
	db.Save(&VulinUser{
		Username: "user1",
		Password: "password123",
		Age:      25,
		Role:     "user",
		Remake:   "我是用户",
	})
	for _, u := range generateRandomUsers(20) {
		db.Save(&u)
	}
	return &dbm{db}, nil
}

func (s *dbm) GetUserBySession(uuid string) (*Session, error) {
	var session Session
	if db := s.db.Where("uuid = ?", uuid).First(&session); db.Error != nil {
		return nil, db.Error
	}
	return &session, nil
}

func (s *dbm) DeleteSession(uuid string) error {
	if db := s.db.Where("uuid = ?", uuid).Delete(&Session{}); db.Error != nil {
		return db.Error
	}
	return nil
}

func (s *dbm) UnsafeSqlQuery(rawString string) ([]map[string]interface{}, error) {
	rows, err := s.db.Debug().Raw(rawString).Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close() // 确保在退出前关闭结果集

	columns, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	var results []map[string]interface{}
	for rows.Next() {
		columnPointers := lo.Map(columns, func(_ string, index int) interface{} {
			var item interface{}
			return &item
		})

		if err := rows.Scan(columnPointers...); err != nil {
			return nil, err
		}
		rowMap := make(map[string]interface{})
		for i, col := range columns {
			rowMap[col] = columnPointers[i]
		}
		results = append(results, rowMap)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}
	return results, nil
}

func (s *dbm) GetUserByIdUnsafe(i string) (map[string]interface{}, error) {
	res, err := s.UnsafeSqlQuery(`select * from vulin_users where id = ` + i + ";")
	if err != nil || len(res) == 0 {
		return nil, err
	}
	return res[0], nil
}

func (s *dbm) GetUserById(i int) (*VulinUser, error) {
	var v VulinUser
	if db := s.db.Where("id = ?", i).First(&v); db.Error != nil {
		return nil, db.Error
	}
	return &v, nil
}

func (s *dbm) GetUserByUsernameUnsafe(i string) ([]map[string]interface{}, error) {
	res, err := s.UnsafeSqlQuery(`select * from vulin_users where username = '` + i + "';")
	if err != nil || len(res) == 0 {
		return nil, err
	}
	return res, nil
}

func (s *dbm) GetUserByUsername(i string) ([]*VulinUser, error) {
	var v []*VulinUser
	db := s.db.Model(&VulinUser{}).Where("username = ?", i).Debug()
	if db := db.Scan(&v); db.Error != nil {
		return nil, db.Error
	}
	return v, nil
}

func (s *dbm) GetUserByUnsafe(i, p string) ([]map[string]interface{}, error) {
	res, err := s.UnsafeSqlQuery(`select * from vulin_users where username = '` + i + `' AND password = '` + p + `';`)
	if err != nil || len(res) == 0 {
		return nil, err
	}
	return res, nil
}

func (s *dbm) GetUser(i, p string) ([]*VulinUser, error) {
	var v []*VulinUser
	db := s.db.Model(&VulinUser{}).Where("username = ? AND password = ?", i, p).Debug()
	if db := db.Scan(&v); db.Error != nil {
		return nil, db.Error
	}
	if len(v) == 0 {
		return nil, utils.Errorf("username or password i\n\treturn v, nilncorrect")
	}
	return v, nil
}

// CreateUser 注册用户
func (s *dbm) CreateUser(user *VulinUser) error {
	// 在这里执行用户创建逻辑，将用户信息存储到数据库
	db := s.db.Create(user)
	if db.Error != nil {
		return db.Error
	}
	return nil
}

// UpdateUser 更新用户信息
func (s *dbm) UpdateUser(user *VulinUser) error {
	// 在这里执行用户更新逻辑，将更新后的用户信息保存到数据库
	db := s.db.Model(&VulinUser{}).Where("id = ?", user.ID).Updates(user)
	if db.Error != nil {
		return db.Error
	}
	return nil
}
