package vulinbox

import "github.com/jinzhu/gorm"

type userManager struct {
	db *gorm.DB
}

func newUserManager(db *gorm.DB) *userManager {
	return &userManager{db: db}
}

func (um *userManager) Authenticate(username, password string) bool {
	var user VulinUser
	if err := um.db.Where("username = ? AND password = ?", username, password).First(&user).Error; err != nil {
		return false
	}
	return true
}

func (um *userManager) IsAdmin(username string) bool {
	var user VulinUser
	if err := um.db.Where("username = ? AND role = ?", username, "admin").First(&user).Error; err != nil {
		return false
	}
	return true
}
