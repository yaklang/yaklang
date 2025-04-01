package vulinbox

import (
	"github.com/jinzhu/gorm"
	"time"
)

type VulinVisitor struct {
	gorm.Model
	Username         string
	Password         string
	Age              int
	LastAccessDomain string
	LastAccessPath   string
	LastAccessTime   time.Time
}
