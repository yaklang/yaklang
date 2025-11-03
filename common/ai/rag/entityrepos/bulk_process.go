package entityrepos

import (
	"context"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/ai/rag/vectorstore"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/asynchelper"
	"github.com/yaklang/yaklang/common/utils/chanx"
)

// AddRequest 封装了单个添加操作的所有信息
type addRequest struct {
	DocID   string
	Content string
	Options []vectorstore.DocumentOption
}

// bulkProcessor 是内部的异步批量处理器
type bulkProcessor struct {
	queue        *chanx.UnlimitedChan[*addRequest]
	batchSize    int
	batchTimeout time.Duration
	collectionMg *vectorstore.SQLiteVectorStoreHNSW
	wg           sync.WaitGroup
	stopCh       chan struct{}
}

// newBulkProcessor 创建一个新的内部处理器
func startBulkProcessor(ctx context.Context, collectionMg *vectorstore.SQLiteVectorStoreHNSW, batchSize int, batchTimeout time.Duration) *bulkProcessor {
	p := &bulkProcessor{
		collectionMg: collectionMg,
		batchSize:    batchSize,
		batchTimeout: batchTimeout,
		queue:        chanx.NewUnlimitedChan[*addRequest](ctx, 10),
		stopCh:       make(chan struct{}),
	}

	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		batch := make([]*addRequest, 0, p.batchSize)
		timer := time.NewTimer(p.batchTimeout)
		if !timer.Stop() {
			<-timer.C
		} // Drain timer
		for {
			select {
			case <-p.stopCh:
				if len(batch) > 0 {
					p.processBatch(ctx, batch)
				}
				return
			case req, ok := <-p.queue.OutputChannel():
				if !ok {
					if len(batch) > 0 {
						p.processBatch(ctx, batch)
					}
					return
				}
				if len(batch) == 0 {
					timer.Reset(p.batchTimeout)
				}
				batch = append(batch, req)
				if len(batch) >= p.batchSize {
					if !timer.Stop() {
						<-timer.C
					} // Drain before processing
					p.processBatch(ctx, batch)
					batch = make([]*addRequest, 0, p.batchSize)
				}
			case <-timer.C:
				if len(batch) > 0 {
					p.processBatch(ctx, batch)
					batch = make([]*addRequest, 0, p.batchSize)
				}
			}
		}
	}()
	log.Infof("Internal bulk processor started")
	return p
}

// stop 优雅地停止处理器
func (p *bulkProcessor) stop() {
	close(p.stopCh) // 发送停止信号
	p.queue.Close() // 关闭队列
	p.wg.Wait()     // 等待所有worker退出
	log.Println("Internal bulk processor stopped gracefully")
}

func (p *bulkProcessor) addRequest(docId string, content string, opts ...vectorstore.DocumentOption) {
	req := &addRequest{
		DocID:   docId,
		Content: content,
		Options: opts,
	}
	p.queue.SafeFeed(req)
}

func (p *bulkProcessor) processBatch(ctx context.Context, batch []*addRequest) {
	log.Infof("[Processor] Processing a batch of %d items.\n", len(batch))
	documents := make([]*vectorstore.Document, 0)
	for _, req := range batch {
		documents = append(documents, vectorstore.BuildDocument(req.DocID, req.Content, req.Options...))
	}
	helper := asynchelper.NewAsyncPerformanceHelper("add index batch", asynchelper.WithCtx(ctx), asynchelper.WithLogRequireTime(1*time.Second), asynchelper.WithTriggerTime(1*time.Second))
	defer helper.Close()

	err := p.collectionMg.Add(documents...)
	if err != nil {
		log.Errorf("[Processor] Failed to add documents: %v.\n", err)
		return
	}
}
