package pcapdb

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
)

// DatabaseHandle is one caller's cancellable lease, sharing a bounded read pool.
// Close releases only this lease. Native Database owners keep their explicit
// lifetime; manager shutdown cancels all leases and closes every pool.
// No embedded Database is exposed, so a caller cannot bypass its task context.
type DatabaseHandle struct {
	ID       string
	database *Database
	parent   context.Context
	ctx      context.Context
	cancel   context.CancelFunc
	closed   atomic.Bool
	mu       sync.Mutex
	stop     func() bool
	onClose  func()
}

func (m *InstanceManager) Acquire(ctx context.Context, identifier string) (*DatabaseHandle, error) {
	if ctx == nil {
		return nil, errors.New("pcapdb: nil handle context")
	}
	// The shared pool was validated on its first open. Each query still checks
	// Ready in its own snapshot; borrowing it needs no repeated quick_check.
	m.mu.Lock()
	cached := m.instances[identifier]
	m.mu.Unlock()
	if cached != nil {
		return m.acquire(ctx, cached)
	}
	database, err := m.open(ctx, identifier, true)
	if err != nil {
		return nil, err
	}
	return m.acquire(ctx, database)
}

func (m *InstanceManager) acquire(parent context.Context, database *Database) (*DatabaseHandle, error) {
	if parent == nil {
		return nil, errors.New("pcapdb: nil handle context")
	}
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return nil, ErrClosed
	}
	if err := parent.Err(); err != nil {
		owned := database.nativeOwner
		m.mu.Unlock()
		if !owned {
			_ = database.closeIdle()
		}
		return nil, err
	}
	// Another lease may have released the last reference between Open/import and
	// acquisition. Reopen under the cache lock rather than returning a dead handle.
	current := m.instances[database.ID]
	if current == nil {
		if len(m.instances) >= 32 {
			m.mu.Unlock()
			return nil, errors.New("pcapdb: 32 open datasets; close unused database handles")
		}
		reader, err := openIndex(database.path, false, false)
		if err != nil {
			m.mu.Unlock()
			return nil, err
		}
		current = &Database{ID: database.ID, path: database.path, manager: m, db: reader}
		m.instances[database.ID] = current
	}
	current.leases++
	ctx, cancel := m.operationContext(parent)
	handle := &DatabaseHandle{ID: current.ID, database: current, parent: parent, ctx: ctx, cancel: cancel}
	m.mu.Unlock()
	handle.mu.Lock()
	handle.stop = context.AfterFunc(ctx, func() { _ = handle.Close() })
	handle.mu.Unlock()
	return handle, nil
}

func (h *DatabaseHandle) Close() error {
	if !h.closed.CompareAndSwap(false, true) {
		return nil
	}
	h.cancel() // unblock this caller's SQL before waiting for any read locks
	h.mu.Lock()
	if h.stop != nil {
		h.stop()
	}
	onClose := h.onClose
	h.mu.Unlock()
	if onClose != nil {
		onClose()
	}
	m, d := h.database.manager, h.database
	m.mu.Lock()
	d.leases--
	closePool := d.leases == 0 && !d.nativeOwner
	m.mu.Unlock()
	if closePool {
		err := d.closeIdle()
		if errors.Is(err, ErrBusy) {
			return nil
		} // a new borrower acquired the pool
		return err
	}
	return nil
}

func (h *DatabaseHandle) check() error {
	if err := h.parent.Err(); err != nil {
		return err
	}
	if h.closed.Load() {
		return ErrClosed
	}
	return h.ctx.Err()
}

func (h *DatabaseHandle) options(options []QueryOption) ([]QueryOption, error) {
	if err := h.check(); err != nil {
		return nil, err
	}
	result := append([]QueryOption(nil), options...)
	return append(result, func(c *queryConfig) error { c.parent = h.ctx; return nil }), nil
}

func (h *DatabaseHandle) Metadata() (*PCAPFileDBMetadata, error) {
	if err := h.check(); err != nil {
		return nil, err
	}
	return h.database.manager.MetadataContext(h.ctx, h.ID)
}
func (h *DatabaseHandle) Export(output string) (string, error) {
	if err := h.check(); err != nil {
		return "", err
	}
	return h.database.manager.Export(h.ctx, h.ID, output)
}
func (h *DatabaseHandle) QueryPackets(options ...QueryOption) ([]Packet, error) {
	options, err := h.options(options)
	if err != nil {
		return nil, err
	}
	return h.database.QueryPackets(options...)
}
func (h *DatabaseHandle) QueryProtocols(options ...QueryOption) ([]ProtocolMessage, error) {
	options, err := h.options(options)
	if err != nil {
		return nil, err
	}
	return h.database.QueryProtocols(options...)
}
func (h *DatabaseHandle) QuerySessions(options ...QueryOption) ([]PCAPSession, error) {
	options, err := h.options(options)
	if err != nil {
		return nil, err
	}
	return h.database.QuerySessions(options...)
}
func (h *DatabaseHandle) PacketSessions(id uint, options ...QueryOption) ([]PCAPSession, error) {
	options, err := h.options(options)
	if err != nil {
		return nil, err
	}
	return h.database.PacketSessions(id, options...)
}
func (h *DatabaseHandle) QueryStreams(options ...QueryOption) ([]PCAPStream, error) {
	options, err := h.options(options)
	if err != nil {
		return nil, err
	}
	return h.database.QueryStreams(options...)
}
func (h *DatabaseHandle) ReadPacket(id uint, options ...QueryOption) ([]byte, error) {
	options, err := h.options(options)
	if err != nil {
		return nil, err
	}
	return h.database.ReadPacket(id, options...)
}
func (h *DatabaseHandle) ReadProtocol(id uint, options ...QueryOption) ([]byte, error) {
	options, err := h.options(options)
	if err != nil {
		return nil, err
	}
	return h.database.ReadProtocol(id, options...)
}
func (h *DatabaseHandle) ProtocolDetails(id uint, options ...QueryOption) (*ProtocolMessage, error) {
	options, err := h.options(options)
	if err != nil {
		return nil, err
	}
	return h.database.ProtocolDetails(id, options...)
}
func (h *DatabaseHandle) ReadStream(id uint, options ...QueryOption) (*StreamPage, error) {
	options, err := h.options(options)
	if err != nil {
		return nil, err
	}
	return h.database.ReadStream(id, options...)
}
func (h *DatabaseHandle) ProtocolPacketIDs(id uint, options ...QueryOption) ([]int64, error) {
	options, err := h.options(options)
	if err != nil {
		return nil, err
	}
	return h.database.ProtocolPacketIDs(id, options...)
}
func (h *DatabaseHandle) StreamChunkPacketIDs(id uint, options ...QueryOption) ([]int64, error) {
	options, err := h.options(options)
	if err != nil {
		return nil, err
	}
	return h.database.StreamChunkPacketIDs(id, options...)
}

func (h *DatabaseHandle) QueryPacketsPage(options ...QueryOption) (*ResultPage[PacketSummary], error) {
	bound, err := h.options(options)
	if err != nil {
		return failedHandlePage[PacketSummary](h, options, "record_id", err)
	}
	return h.database.QueryPacketsPage(bound...)
}

func (h *DatabaseHandle) QueryProtocolsPage(options ...QueryOption) (*ResultPage[ProtocolSummary], error) {
	bound, err := h.options(options)
	if err != nil {
		return failedHandlePage[ProtocolSummary](h, options, "record_id", err)
	}
	return h.database.QueryProtocolsPage(bound...)
}

func (h *DatabaseHandle) QuerySessionsPage(options ...QueryOption) (*ResultPage[SessionSummary], error) {
	bound, err := h.options(options)
	if err != nil {
		return failedHandlePage[SessionSummary](h, options, "record_id", err)
	}
	return h.database.QuerySessionsPage(bound...)
}

func (h *DatabaseHandle) QueryStreamsPage(options ...QueryOption) (*ResultPage[StreamSummary], error) {
	bound, err := h.options(options)
	if err != nil {
		return failedHandlePage[StreamSummary](h, options, "record_id", err)
	}
	return h.database.QueryStreamsPage(bound...)
}

func (h *DatabaseHandle) ReadPacketPage(id uint, options ...QueryOption) (*ResultPage[DataPreview], error) {
	bound, err := h.options(options)
	if err != nil {
		return failedHandlePage[DataPreview](h, options, "record_id", err)
	}
	return h.database.ReadPacketPage(id, bound...)
}

func (h *DatabaseHandle) ReadProtocolPage(id uint, options ...QueryOption) (*ResultPage[DataPreview], error) {
	bound, err := h.options(options)
	if err != nil {
		return failedHandlePage[DataPreview](h, options, "record_id", err)
	}
	return h.database.ReadProtocolPage(id, bound...)
}

func (h *DatabaseHandle) ReadStreamPage(id uint, options ...QueryOption) (*ResultPage[DataPreview], error) {
	bound, err := h.options(options)
	if err != nil {
		return failedHandlePage[DataPreview](h, options, "chunk_id", err)
	}
	return h.database.ReadStreamPage(id, bound...)
}

func (h *DatabaseHandle) ProtocolDetailsPage(id uint, options ...QueryOption) (*ResultPage[ProtocolDetail], error) {
	bound, err := h.options(options)
	if err != nil {
		return failedHandlePage[ProtocolDetail](h, options, "record_id", err)
	}
	return h.database.ProtocolDetailsPage(id, bound...)
}

func (h *DatabaseHandle) DiscoverProtocolFields(options ...QueryOption) (*ResultPage[ProtocolField], error) {
	bound, err := h.options(options)
	if err != nil {
		return failedHandlePage[ProtocolField](h, options, "message_id", err)
	}
	return h.database.DiscoverProtocolFields(bound...)
}

func (h *DatabaseHandle) ProtocolFieldIndexes(options ...QueryOption) ([]ProtocolFieldIndex, error) {
	options, err := h.options(options)
	if err != nil {
		return nil, err
	}
	return h.database.ProtocolFieldIndexes(options...)
}
func (h *DatabaseHandle) EnsureProtocolFieldIndexes(paths ...string) ([]ProtocolFieldIndex, error) {
	if err := h.check(); err != nil {
		return nil, err
	}
	return h.database.manager.EnsureProtocolFieldIndexes(h.ctx, h.ID, paths...)
}
func (h *DatabaseHandle) RebuildAnalysis(options ...ImportOption) (*DatabaseHandle, error) {
	if err := h.check(); err != nil {
		return nil, err
	}
	parent := func(c *importConfig) error { c.parent = h.ctx; c.leased = true; return nil }
	_, err := h.database.manager.RebuildAnalysis(h.ID, append(options, parent)...)
	if err != nil {
		return nil, err
	}
	return h, nil
}

func (h *DatabaseHandle) ExportPacket(id uint, output string, options ...QueryOption) (*PayloadArtifact, error) {
	options, err := h.options(options)
	if err != nil {
		return nil, err
	}
	return h.database.ExportPacket(id, output, options...)
}

func (h *DatabaseHandle) ExportProtocol(id uint, output string, options ...QueryOption) (*PayloadArtifact, error) {
	options, err := h.options(options)
	if err != nil {
		return nil, err
	}
	return h.database.ExportProtocol(id, output, options...)
}

func (h *DatabaseHandle) ExportProtocolFields(id uint, output string, options ...QueryOption) (*PayloadArtifact, error) {
	options, err := h.options(options)
	if err != nil {
		return nil, err
	}
	return h.database.ExportProtocolFields(id, output, options...)
}

func (h *DatabaseHandle) ExportStream(id uint, output string, options ...QueryOption) (*PayloadArtifact, error) {
	options, err := h.options(options)
	if err != nil {
		return nil, err
	}
	return h.database.ExportStream(id, output, options...)
}

func (h *DatabaseHandle) ProtocolPacketIDsPage(id uint, options ...QueryOption) (*ResultPage[RecordReference], error) {
	bound, err := h.options(options)
	if err != nil {
		return failedHandlePage[RecordReference](h, options, "record_id", err)
	}
	return h.database.ProtocolPacketIDsPage(id, bound...)
}

func (h *DatabaseHandle) StreamChunkPacketIDsPage(id uint, options ...QueryOption) (*ResultPage[RecordReference], error) {
	bound, err := h.options(options)
	if err != nil {
		return failedHandlePage[RecordReference](h, options, "record_id", err)
	}
	return h.database.StreamChunkPacketIDsPage(id, bound...)
}

func (h *DatabaseHandle) PacketSessionsPage(id uint, options ...QueryOption) (*ResultPage[SessionSummary], error) {
	bound, err := h.options(options)
	if err != nil {
		return failedHandlePage[SessionSummary](h, options, "record_id", err)
	}
	return h.database.PacketSessionsPage(id, bound...)
}

func failedHandlePage[T any](h *DatabaseHandle, options []QueryOption, kind string, err error) (*ResultPage[T], error) {
	c := &queryConfig{ctx: context.Background()}
	for _, option := range options {
		if option != nil {
			if option(c) != nil {
				break
			}
		}
	}
	return failPage(emptyPage[T](h.ID, c.after, kind), err)
}
