package vulinbox

import (
	"github.com/jinzhu/gorm"
	_ "github.com/mattn/go-sqlite3"
	"github.com/yaklang/yaklang/common/consts"
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
	fp.Close()
	db, err := gorm.Open("sqlite3", name)
	if err != nil {
		return nil, err
	}
	db.AutoMigrate(&VulinUser{})
	db.Save(&VulinUser{
		Username: "admin",
		Password: "password",
		Age:      25,
	})
	db.Save(&VulinUser{
		Username: "root",
		Password: "p@ssword",
		Age:      25,
	})
	db.Save(&VulinUser{
		Username: "user1",
		Password: "password123",
		Age:      25,
	})
	db.Save(&VulinUser{
		Username: "admin",
		Password: "123456",
		Age:      25,
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
