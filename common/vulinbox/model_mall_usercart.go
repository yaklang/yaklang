package vulinbox

import (
	"github.com/jinzhu/gorm"
	// uuid "github.com/satori/go.uuid"
)

type UserCart struct {
	gorm.Model
	UserID          int     `gorm:"column:UserID"`          //用户ID
	ProductName     string  `gorm:"column:ProductName"`     //商品名称
	Description     string  `gorm:"column:Description"`     //商品描述
	ProductPrice    float64 `gorm:"column:ProductPrice"`    //商品价格
	ProductQuantity int     `gorm:"column:ProductQuantity"` //商品数量
	TotalPrice      float64 `gorm:"column:TotalPrice"`      //商品总价
}

// 加入购物车
func (s *dbm) AddCart(UserID int, cart UserCart) (err error) {
	cart.UserID = UserID
	cart.ProductQuantity = 1 // 设置默认的商品数量为1
	if err := s.db.Create(&cart).Error; err != nil {
		return err
	}
	return nil
}

// 购物车商品数量加一
func (s *dbm) AddCartQuantity(UserID int, ProductName string) (err error) {
	var v UserCart
	v.UserID = UserID
	v.ProductName = ProductName
	if err := s.db.Model(&v).Where("UserID = ? AND ProductName = ?", v.UserID, v.ProductName).Update("ProductQuantity", gorm.Expr("ProductQuantity + ?", 1)).Error; err != nil {
		return err
	}
	return nil
}

// 购物车商品数量减一，当ProductQuantity数量减为0时，删除该条商品记录
func (s *dbm) SubCartQuantity(UserID int, ProductName string) (err error) {
	var v UserCart
	v.UserID = UserID
	v.ProductName = ProductName
	if err := s.db.Model(&v).Where("UserID = ? AND ProductName = ?", v.UserID, v.ProductName).Update("ProductQuantity", gorm.Expr("ProductQuantity - ?", 1)).Error; err != nil {
		return err
	}
	//查询当前商品数量
	if err := s.db.Where("UserID = ? AND ProductName = ?", v.UserID, v.ProductName).First(&v).Error; err != nil {
		return err
	}
	//如果当前商品数量为0，删除该条商品记录

	if v.ProductQuantity == 0 {
		if err := s.db.Where("UserID = ? AND ProductName = ?", v.UserID, v.ProductName).Delete(&v).Error; err != nil {
			return err
		}
	}
	return nil
}

// 获取购物车
func (s *dbm) GetCart(UserID int) (userCart []UserCart, err error) {
	var v UserCart
	v.UserID = UserID
	if err := s.db.Where("UserID = ?", v.UserID).Find(&userCart).Error; err != nil {
		return nil, err
	}
	return userCart, nil
}

// 获取购物车商品总数
func (s *dbm) GetUserCartCount(UserID int) (count int64, err error) {
	var total struct {
		Total int64
	}
	if err := s.db.Raw("SELECT SUM(ProductQuantity) AS total FROM user_carts WHERE UserID = ?", UserID).Scan(&total).Error; err != nil {
		return 0, err
	}
	return total.Total, nil
}

// 通过user_id和商品名称删除购物车
func (s *dbm) DeleteCartByName(UserID int, ProductName string) (err error) {
	var v UserCart
	v.UserID = UserID
	v.ProductName = ProductName
	if err := s.db.Where("UserID = ? AND ProductName = ?", v.UserID, v.ProductName).Delete(&v).Error; err != nil {
		return err
	}
	return nil
}

// 检车购物车是否存在
func (s *dbm) CheckCart(UserID int, ProductName string) (bool, error) {
	var v UserCart
	v.UserID = UserID
	v.ProductName = ProductName
	err := s.db.Where("UserID = ? AND ProductName = ?", v.UserID, v.ProductName).First(&v).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			// 没有找到匹配的记录，返回 false
			return false, nil
		}
		// 查询过程中出现了错误，返回错误
		return false, err
	}
	// 找到了匹配的记录，返回 true
	return true, nil
}
