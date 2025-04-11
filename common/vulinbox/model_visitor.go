package vulinbox

import (
	"time"

	"github.com/jinzhu/gorm"
)

type VulinVisitor struct {
	gorm.Model
	Username         string
	Password         string
	Age              int
	LastAccessDomain string
	LastAccessPath   string
	LastAccessTime   time.Time
	ProxyIp          string
	RealIp           string
}
