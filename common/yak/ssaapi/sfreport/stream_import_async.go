package sfreport

import (
	"context"
	"sync"
	"time"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/asyncdb"
)

// StreamImportConfig controls batching behaviour for AsyncStreamImporter.
type StreamImportConfig struct {
	BatchSize     int           // batch size per flush
	FlushInterval time.Duration // max wait before flushing a partial batch
	ChannelBuffer int           // channel buffer size (back-pressure)
}

const maxImportErrors = 500

func DefaultStreamImportConfig() *StreamImportConfig {
	return &StreamImportConfig{
		BatchSize:     200,
		FlushInterval: 200 * time.Millisecond,
		ChannelBuffer: 20000,
	}
}

// AsyncStreamImporter batches file and risk writes using asyncdb.Save.
type AsyncStreamImporter struct {
	db          *gorm.DB
	programName string
	config      *StreamImportConfig

	fileSaver *asyncdb.Save[*File]
	riskSaver *asyncdb.Save[*riskWithDataflow]

	ctx    context.Context
	cancel context.CancelFunc

	mu           sync.Mutex
	filesWritten int
	risksWritten int
	errors       []error
}

type riskWithDataflow struct {
	risk     *Risk
	dataflow *DataFlowPath
}

func NewAsyncStreamImporter(db *gorm.DB, programName string, config *StreamImportConfig) *AsyncStreamImporter {
	if config == nil {
		config = DefaultStreamImportConfig()
	}

	ctx, cancel := context.WithCancel(context.Background())

	importer := &AsyncStreamImporter{
		db:          db,
		programName: programName,
		config:      config,
		ctx:         ctx,
		cancel:      cancel,
	}

	importer.fileSaver = asyncdb.NewSave(
		func(files []*File) {
			start := time.Now()
			importer.batchSaveFiles(db, files)
			if len(files) > 0 {
				duration := time.Since(start)
				importer.mu.Lock()
				total := importer.filesWritten
				importer.mu.Unlock()
				log.Infof("[Perf] Files: saved batch=%d, total=%d, took=%v, rate=%.1f/s",
					len(files), total, duration, float64(len(files))/duration.Seconds())
			}
		},
		asyncdb.WithContext(ctx),
		asyncdb.WithSaveSize(config.BatchSize),
		asyncdb.WithSaveTimeout(config.FlushInterval),
		asyncdb.WithName("SSAFiles"),
	)

	importer.riskSaver = asyncdb.NewSave(
		func(risks []*riskWithDataflow) {
			start := time.Now()
			importer.batchSaveRisks(db, risks)
			if len(risks) > 0 {
				duration := time.Since(start)
				importer.mu.Lock()
				total := importer.risksWritten
				importer.mu.Unlock()
				log.Infof("[Perf] Risks: saved batch=%d, total=%d, took=%v, rate=%.1f/s",
					len(risks), total, duration, float64(len(risks))/duration.Seconds())
			}
		},
		asyncdb.WithContext(ctx),
		asyncdb.WithSaveSize(config.BatchSize),
		asyncdb.WithSaveTimeout(config.FlushInterval),
		asyncdb.WithName("SSARisks"),
	)

	return importer
}

func (i *AsyncStreamImporter) batchSaveFiles(db *gorm.DB, files []*File) {
	if len(files) == 0 {
		return
	}
	successCount := 0
	utils.GormTransaction(db, func(tx *gorm.DB) error {
		for _, file := range files {
			if err := file.SaveToDB(tx, i.programName); err != nil {
				i.recordError(utils.Wrapf(err, "save file %s failed", file.Path))
				continue
			}
			successCount++
		}
		return nil
	})
	i.mu.Lock()
	i.filesWritten += successCount
	i.mu.Unlock()
}

func (i *AsyncStreamImporter) batchSaveRisks(db *gorm.DB, risks []*riskWithDataflow) {
	if len(risks) == 0 {
		return
	}
	successCount := 0
	utils.GormTransaction(db, func(tx *gorm.DB) error {
		for _, item := range risks {
			riskHash, err := item.risk.SaveToDB(tx)
			if err != nil {
				i.recordError(utils.Wrapf(err, "save risk %s failed", item.risk.Title))
				continue
			}
			successCount++
			if item.dataflow != nil {
				saver := NewSaveDataFlowCtx(tx, riskHash)
				saver.SaveDataFlow(item.dataflow)
			}
		}
		return nil
	})
	i.mu.Lock()
	i.risksWritten += successCount
	i.mu.Unlock()
}

func (i *AsyncStreamImporter) AddFile(file *File) error {
	if i.fileSaver == nil {
		return utils.Errorf("file saver not initialized")
	}
	i.fileSaver.Save(file)
	return nil
}

func (i *AsyncStreamImporter) AddRisk(risk *Risk, dataflow *DataFlowPath) error {
	if i.riskSaver == nil {
		return utils.Errorf("risk saver not initialized")
	}
	i.riskSaver.Save(&riskWithDataflow{risk: risk, dataflow: dataflow})
	return nil
}

func (i *AsyncStreamImporter) recordError(err error) {
	i.mu.Lock()
	defer i.mu.Unlock()
	if len(i.errors) < maxImportErrors {
		i.errors = append(i.errors, err)
	}
	log.Warnf("%v", err)
}

func (i *AsyncStreamImporter) Close() error {
	if i.fileSaver != nil {
		i.fileSaver.Close()
	}
	if i.riskSaver != nil {
		i.riskSaver.Close()
	}
	i.cancel()

	i.mu.Lock()
	defer i.mu.Unlock()
	log.Infof("AsyncStreamImporter closed: files=%d, risks=%d, errors=%d",
		i.filesWritten, i.risksWritten, len(i.errors))
	return nil
}

func (i *AsyncStreamImporter) GetStats() (filesWritten, risksWritten, errorsCount int) {
	i.mu.Lock()
	defer i.mu.Unlock()
	return i.filesWritten, i.risksWritten, len(i.errors)
}

func (i *AsyncStreamImporter) GetErrors() []error {
	i.mu.Lock()
	defer i.mu.Unlock()
	return append([]error{}, i.errors...)
}
