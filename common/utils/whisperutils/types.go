package whisperutils

// WhisperResponse is the top-level structure for the entire JSON output.
type WhisperResponse struct {
	Task     string    `json:"task"`
	Language string    `json:"language"`
	Duration float64   `json:"duration"`
	Text     string    `json:"text"`
	Segments []Segment `json:"segments"`
}

// Segment represents a continuous chunk of speech. Natural for subtitle entries.
type Segment struct {
	ID    int     `json:"id"`
	Text  string  `json:"text"`
	Start float64 `json:"start"`
	End   float64 `json:"end"`
	Words []Word  `json:"words"`
}

// Word contains the precise timing and text for a single word.
type Word struct {
	Word        string  `json:"word"`
	Start       float64 `json:"start"`
	End         float64 `json:"end"`
	Probability float64 `json:"probability"`
}
