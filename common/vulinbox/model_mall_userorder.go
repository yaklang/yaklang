package vulinbox

import (
	// "time"
	"github.com/jinzhu/gorm"
	// uuid "github.com/satori/go.uuid"
)

type UserOrder struct {
	gorm.Model
	UserID         int     `gorm:"column:UserID"`         //用户ID
	ProductName    string  `gorm:"column:ProductName"`    //商品名称
	Quantity       int     `gorm:"column:Quantity"`       //数量
	TotalPrice     float64 `gorm:"column:TotalPrice"`     //总价
	DeliveryStatus string  `gorm:"column:DeliveryStatus"` //发货状态

}

// 提交订单
func (s *dbm) AddOrder(UserID int, order UserOrder) (err error) {
	order.UserID = UserID
	if err := s.db.Create(&order).Error; err != nil {
		return err
	}
	return nil
}

// 更新发货状态
// func (s *dbm) UpdateDeliveryStatus(UserID int, ProductName string, DeliveryStatus string) (err error) {
// 	var v UserOrder
// 	v.UserID = UserID
// 	v.ProductName = ProductName
// 	if err := s.db.Model(&v).Where("UserID = ? AND ProductName = ?", v.UserID, v.ProductName).Update("DeliveryStatus", DeliveryStatus).Error; err != nil {
// 		return err
// 	}
// 	return nil
// }

// 查询订单
func (s *dbm) QueryOrder(UserID int) (order []UserOrder, err error) {
	if err := s.db.Where("UserID = ?", UserID).Find(&order).Error; err != nil {
		return nil, err
	}
	return order, nil
}

// 删除订单
func (s *dbm) DeleteOrder(UserID int, ProductName string) (err error) {
	var v UserOrder
	v.UserID = UserID
	v.ProductName = ProductName
	if err := s.db.Where("UserID = ? AND ProductName = ?", v.UserID, v.ProductName).Delete(&v).Error; err != nil {
		return err
	}
	return nil
}
