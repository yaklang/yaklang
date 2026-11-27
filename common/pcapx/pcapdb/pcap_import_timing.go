package pcapdb

import (
	"fmt"
	"os"
	"sync/atomic"
	"time"
)

type importTiming struct {
	enabled bool
	start   time.Time

	setup               atomic.Int64
	ingest              atomic.Int64
	captureRead         atomic.Int64
	packetMetadata      atomic.Int64
	analyzerFeed        atomic.Int64
	protocolCallback    atomic.Int64
	protocolAssemble    atomic.Int64
	analysisClose       atomic.Int64
	dbFlush             atomic.Int64
	packetInsert        atomic.Int64
	interfaceInsert     atomic.Int64
	packetRowInsert     atomic.Int64
	captureRecordInsert atomic.Int64
	sessionInsert       atomic.Int64
	messageInsert       atomic.Int64
	referenceInsert     atomic.Int64
	manifestWrite       atomic.Int64
	transaction         atomic.Int64
	catalog             atomic.Int64
	finalIndexes        atomic.Int64
	finalize            atomic.Int64
	flushCount          atomic.Int64
}

func newImportTiming() *importTiming {
	return &importTiming{enabled: os.Getenv("YAK_PCAPDB_IMPORT_TIMING") == "1", start: time.Now()}
}

func (t *importTiming) begin() time.Time {
	if t != nil && t.enabled {
		return time.Now()
	}
	return time.Time{}
}

func (t *importTiming) add(counter *atomic.Int64, since time.Time) {
	if t != nil && t.enabled && !since.IsZero() {
		counter.Add(int64(time.Since(since)))
	}
}

func (t *importTiming) duration(counter *atomic.Int64) time.Duration {
	return time.Duration(counter.Load())
}

func (t *importTiming) report() {
	if t == nil || !t.enabled {
		return
	}
	analyzerExclusive := t.duration(&t.analyzerFeed) - t.duration(&t.protocolCallback)
	ingestNonAnalyzer := t.duration(&t.ingest) - t.duration(&t.analyzerFeed)
	fmt.Fprintf(os.Stderr,
		"pcapdb import timing total=%s flush_count=%d setup=%s ingest=%s ingest_non_analyzer=%s capture_read=%s packet_metadata=%s analyzer_feed=%s analyzer_exclusive=%s protocol_callback=%s protocol_assemble=%s analysis_close=%s db_flush=%s capture_insert=%s interface_insert=%s packet_row_insert=%s capture_record_insert=%s session_insert=%s message_insert=%s reference_insert=%s manifest_write=%s commit=%s catalog=%s final_indexes=%s finalize=%s\n",
		time.Since(t.start), t.flushCount.Load(),
		t.duration(&t.setup), t.duration(&t.ingest),
		ingestNonAnalyzer, t.duration(&t.captureRead), t.duration(&t.packetMetadata), t.duration(&t.analyzerFeed),
		analyzerExclusive, t.duration(&t.protocolCallback), t.duration(&t.protocolAssemble), t.duration(&t.analysisClose),
		t.duration(&t.dbFlush), t.duration(&t.packetInsert), t.duration(&t.interfaceInsert),
		t.duration(&t.packetRowInsert), t.duration(&t.captureRecordInsert), t.duration(&t.sessionInsert),
		t.duration(&t.messageInsert), t.duration(&t.referenceInsert), t.duration(&t.manifestWrite),
		t.duration(&t.transaction), t.duration(&t.catalog), t.duration(&t.finalIndexes), t.duration(&t.finalize))
}
