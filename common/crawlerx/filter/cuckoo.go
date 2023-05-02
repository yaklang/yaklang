package filter

import "github.com/yaklang/yaklang/common/cuckoo"

// 默认参数的过滤容器, 全局使用一个容易造成碰撞, 因此拆分成四个
// NewGenericCuckoo 极限容量约 400 万
func NewGenericCuckoo() *cuckoo.Filter {
	return cuckoo.New(
		cuckoo.BucketEntries(18),
		cuckoo.BucketTotal(1<<18),
		cuckoo.Kicks(300),
	)
}

// NewPathCuckoo 极限容量约 200 万
func NewPathCuckoo() *cuckoo.Filter {
	return cuckoo.New(cuckoo.BucketEntries(16),
		cuckoo.BucketTotal(1<<17),
		cuckoo.Kicks(300))
}

// NewDirCuckoo 极限容量约 100 万
func NewDirCuckoo() *cuckoo.Filter {
	return cuckoo.New(cuckoo.BucketEntries(14),
		cuckoo.BucketTotal(1<<16),
		cuckoo.Kicks(300))
}

// NewWebsiteCuckoo 极限容量约 40 万
func NewWebsiteCuckoo() *cuckoo.Filter {
	return cuckoo.New(cuckoo.BucketEntries(12),
		cuckoo.BucketTotal(1<<15),
		cuckoo.Kicks(300))
}
