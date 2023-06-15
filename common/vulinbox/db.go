package vulinbox

import (
	"github.com/jinzhu/gorm"
	_ "github.com/mattn/go-sqlite3"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
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
	db, err := gorm.Open("sqlite3", name)
	if err != nil {
		return nil, err
	}
	db.AutoMigrate(&VulinUser{})
	db.AutoMigrate(&Session{})
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

func (s *dbm) GetUserById(i int) (*VulinUser, error) {
	var v VulinUser
	if db := s.db.Where("id = ?", i).First(&v); db.Error != nil {
		return nil, db.Error
	}
	return &v, nil
}

func (s *dbm) GetUserBySession(uuid string) (*Session, error) {
	var session Session
	if db := s.db.Where("uuid = ?", uuid).First(&session); db.Error != nil {
		return nil, db.Error
	}
	return &session, nil
}

func (s *dbm) GetUserByIdUnsafe(i string) (*VulinUser, error) {
	var v VulinUser
	db := s.db.Raw(`select * from vulin_users where id = ` + i + ";").Debug()
	if db := db.Scan(&v); db.Error != nil {
		return nil, db.Error
	}
	return &v, nil
}

func (s *dbm) GetUserByUsernameUnsafe(i string) ([]*VulinUser, error) {
	var v []*VulinUser
	db := s.db.Raw(`select * from vulin_users where username = '` + i + "';").Debug()
	if db := db.Scan(&v); db.Error != nil {
		return nil, db.Error
	}
	return v, nil
}

func (s *dbm) GetUserByUnsafe(i, p string) ([]*VulinUser, error) {
	var v []*VulinUser
	sql := `select * from vulin_users where username = '` + i + `' AND password = '` + p + `';`
	db := s.db.Raw(sql).Debug()
	if db := db.Scan(&v); db.Error != nil {
		return nil, db.Error
	}
	if len(v) == 0 {
		return nil, utils.Errorf("username or password incorrect")
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
