package ssadb

type IrNamePool struct {
	ProgramName string `json:"program_name" gorm:"index;not null"`
	NameID      int64  `json:"name_id" gorm:"primary_key;auto_increment"`
	Name        string `json:"name" gorm:"index;not null"`
}

func (i *IrNamePool) TableName() string {
	return TableIrNamePool
}
