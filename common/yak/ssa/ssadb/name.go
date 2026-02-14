package ssadb

type IrNamePool struct {
	ProgramName string `json:"program_name" gorm:"index;not null"`
	NameID      int64  `json:"name_id" gorm:"primary_key;auto_increment"`
	Name        string `json:"name" gorm:"index;not null;unique_index:idx_ir_name_pool_program_name_name"`
}

func (i *IrNamePool) TableName() string {
	return "ir_name_pool"
}
