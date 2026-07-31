package aicommon

import (
	"context"

	"github.com/yaklang/yaklang/common/schema"
)

// ResultReceipt is the platform-neutral acknowledgement returned after a
// structured result has been accepted by the configured backend.
type ResultReceipt struct {
	ResultID  string
	DedupeKey string
	BackendID string
}

// AssetResult is a platform-neutral structured asset produced by a Focus Mode.
// Payload is an inline JSON document whose schema is owned by Kind.
type AssetResult struct {
	Kind        string
	Title       string
	Target      string
	IdentityKey string
	Payload     []byte
}

// ResultSink is injected per AI runtime. Desktop runtimes leave it unset and
// retain their existing local persistence behavior; SaaS runtimes inject a
// sink that transfers ownership to Legion.
type ResultSink interface {
	SubmitRisk(context.Context, *schema.Risk) (ResultReceipt, error)
}

// AssetResultSink is an optional capability implemented by backends that can
// take ownership of structured assets. Keeping it separate preserves
// compatibility with ResultSink implementations that only accept risks.
type AssetResultSink interface {
	ResultSink
	SubmitAsset(context.Context, AssetResult) (ResultReceipt, error)
}

// ResultSinkProvider is intentionally separate from AICallerConfigIf so
// existing third-party and test implementations of that broad interface do
// not need to change.
type ResultSinkProvider interface {
	GetResultSink() ResultSink
}

func ResultSinkFromConfig(config any) ResultSink {
	provider, ok := config.(ResultSinkProvider)
	if !ok || provider == nil {
		return nil
	}
	return provider.GetResultSink()
}

func AssetResultSinkFromConfig(config any) AssetResultSink {
	sink, ok := ResultSinkFromConfig(config).(AssetResultSink)
	if !ok {
		return nil
	}
	return sink
}

// ResultSinkFunc is a compact adapter for tests and lightweight runtimes.
type ResultSinkFunc func(context.Context, *schema.Risk) (ResultReceipt, error)

func (f ResultSinkFunc) SubmitRisk(
	ctx context.Context,
	risk *schema.Risk,
) (ResultReceipt, error) {
	return f(ctx, risk)
}
