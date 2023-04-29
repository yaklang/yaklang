package partition

import (
	"github.com/jinzhu/gorm"
)

type Seperator struct {
	DB      *gorm.DB
	Origins []string
}

func (s *Seperator) List() (result []string) {
	for _, origin := range s.Origins {
		//for l := range SeperateDict(origin, s.GetProfileDatabase() {
		for f := range SeperateIPv4nPort(origin) {
			result = append(result, f)
		}
		//}
	}
	return
}

func NewSeperator(origins ...string) *Seperator {
	return &Seperator{Origins: origins}
}

func NewSeperatorWithDB(d *gorm.DB, origins ...string) *Seperator {
	return &Seperator{
		DB:      d,
		Origins: origins,
	}
}
