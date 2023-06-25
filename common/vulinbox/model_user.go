package vulinbox

import (
	"github.com/jinzhu/gorm"
	"math/rand"
	"time"
)

type VulinUser struct {
	gorm.Model

	Username string
	Password string
	Age      int

	Role string // 添加角色字段

	Remake string // 添加备注字段

}

// 生成指定数量的随机用户数据
func generateRandomUsers(count int) []VulinUser {
	// 定义可选的用户名和密码字符
	const chars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

	// 初始化随机数生成器
	rand.Seed(time.Now().UnixNano())

	// 生成测试数据
	users := make([]VulinUser, count)
	for i := 0; i < count; i++ {
		// 生成随机的用户名和密码
		username := generateRandomString(chars, 8)
		password := generateRandomString(chars, 12)

		// 生成随机的年龄（18-65岁之间）
		age := rand.Intn(48) + 18

		// 创建用户实例并将其添加到用户列表中
		users[i] = VulinUser{
			Username: username,
			Password: password,
			Age:      age,
			Role:     "user",
			Remake:   "我是用户",
		}
	}

	return users
}

// 生成指定长度的随机字符串
func generateRandomString(chars string, length int) string {
	b := make([]byte, length)
	for i := range b {
		b[i] = chars[rand.Intn(len(chars))]
	}
	return string(b)
}
