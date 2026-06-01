package knowledgebench

// CompressOptions configures the CompressLongTextWithDestination behavior.
// All fields default to production values when zero.
type CompressOptions struct {
	MaxChunkSizeBytes int     // default 80*1024
	MaxChunks         int     // default 20
	ScoreThreshold    float64 // default 0.3
	TargetTokenSize   int64   // overrides the parameter
}

// DefaultCompressOptions returns the production-default values.
func DefaultCompressOptions() CompressOptions {
	return CompressOptions{
		MaxChunkSizeBytes: 80 * 1024,
		MaxChunks:         20,
		ScoreThreshold:    0.3,
		TargetTokenSize:   10 * 1024,
	}
}

// Merge applies non-zero fields from o onto defaults.
func (o CompressOptions) Merge(defaults CompressOptions) CompressOptions {
	if o.MaxChunkSizeBytes <= 0 {
		o.MaxChunkSizeBytes = defaults.MaxChunkSizeBytes
	}
	if o.MaxChunks <= 0 {
		o.MaxChunks = defaults.MaxChunks
	}
	if o.ScoreThreshold <= 0 {
		o.ScoreThreshold = defaults.ScoreThreshold
	}
	if o.TargetTokenSize <= 0 {
		o.TargetTokenSize = defaults.TargetTokenSize
	}
	return o
}
