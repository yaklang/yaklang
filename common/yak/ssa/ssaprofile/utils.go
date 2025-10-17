package ssaprofile

// 辅助函数：将字节转换为兆字节 (MB)
func bToMb(b uint64) uint64 {
	return b / 1024 / 1024
}
