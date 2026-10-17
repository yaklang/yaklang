package vectorstore

import (
	"fmt"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/schema"
)

func BenchmarkGraphPersistence(b *testing.B) {
	for _, size := range []int{256, 1024} {
		for _, incremental := range []bool{false, true} {
			b.Run(fmt.Sprintf("nodes=%d/incremental=%t", size, incremental), func(b *testing.B) {
				threshold := int(^uint(0) >> 1)
				if incremental {
					threshold = 0
				}
				db, store := persistTestStore(b, 32, WithGraphPersistence(threshold, time.Hour))
				docs := make([]*Document, size)
				for i := range docs {
					docs[i] = &Document{ID: fmt.Sprintf("seed-%d", i), Content: fmt.Sprintf("seed-%d", i)}
				}
				if err := store.Add(docs...); err != nil {
					b.Fatal(err)
				}
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					id := fmt.Sprintf("insert-%d", i)
					if err := store.AddWithOptions(id, id); err != nil {
						b.Fatal(err)
					}
				}
				b.StopTimer()
				if incremental {
					var delta schema.VectorStoreGraphDelta
					if err := db.Order("seq desc").First(&delta).Error; err != nil {
						b.Fatal(err)
					}
					b.ReportMetric(float64(len(delta.Payload)), "persisted-bytes/op")
				} else {
					var collection schema.VectorStoreCollection
					if err := db.First(&collection, store.collection.ID).Error; err != nil {
						b.Fatal(err)
					}
					b.ReportMetric(float64(len(collection.GraphBinary)), "persisted-bytes/op")
				}
			})
		}
	}
}
