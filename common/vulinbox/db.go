package vulinbox

import (
	"github.com/jinzhu/gorm"
	_ "github.com/mattn/go-sqlite3"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
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
	db.Save(&VulinUser{
		Username: "admin",
		Password: "admin",
		Age:      25,
		Role:     "admin",
	})
	db.Save(&VulinUser{
		Username: "root",
		Password: "p@ssword",
		Age:      25,
		Role:     "admin",
	})
	db.Save(&VulinUser{
		Username: "user1",
		Password: "password123",
		Age:      25,
		Role:     "user",
	})
	for _, u := range generateRandomUsers(200) {
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
