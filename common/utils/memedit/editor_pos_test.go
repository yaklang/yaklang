package memedit

import (
	"testing"
)

func TestGetOffsetByPosition_TABLE(t *testing.T) {
	editor := NewMemEditor("0123456789\n0123456789\n0123456789\n0123456789\n0123456789\n0123456789")

	tests := []struct {
		name      string
		line, col int
		want      int
		wantErr   bool
	}{
		{
			name:    "Start of first line",
			line:    1,
			col:     0,
			want:    0,
			wantErr: false,
		},
		{
			name:    "End of first line",
			line:    1,
			col:     10,
			want:    10,
			wantErr: false,
		},
		{
			name:    "Start of second line",
			line:    2,
			col:     0,
			want:    11,
			wantErr: false,
		},
		{
			name:    "Invalid negative line number",
			line:    -1,
			col:     0,
			want:    0,
			wantErr: true,
		},
		{
			name:    "Invalid negative column number",
			line:    1,
			col:     -1,
			want:    0,
			wantErr: true,
		},
		{
			name:    "Column number out of range",
			line:    1,
			col:     15,
			want:    10, // Should return the end of the line offset
			wantErr: false,
		},
		{
			name:    "Line number out of range",
			line:    10,
			col:     0,
			want:    65, // Should return the last valid offset
			wantErr: true,
		},
		{
			name:    "Last valid position",
			line:    6,
			col:     10,
			want:    65,
			wantErr: false,
		},
		{
			name:    "First position of last line",
			line:    6,
			col:     0,
			want:    55,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := editor.GetOffsetByPositionWithError(tt.line, tt.col+1)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetOffsetByPositionWithError() expected error = %v, got error = %v", tt.wantErr, err)
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("GetOffsetByPositionWithError() expected = %d, got = %d", tt.want, got)
			}
		})
	}
}

func TestGetOffsetByPositionWithError_Table3(t *testing.T) {
	editor := NewMemEditor("0123456789\n0123456789\n\n0123456789\n0123456789\n0123456789\n0123456789")

	tests := []struct {
		name      string
		line, col int
		want      int
		wantErr   bool
	}{
		{
			name:    "Start of first line",
			line:    1,
			col:     0,
			want:    0,
			wantErr: false,
		},
		{
			name:    "End of first line",
			line:    1,
			col:     10,
			want:    10,
			wantErr: false,
		},
		{
			name:    "Start of second line",
			line:    2,
			col:     0,
			want:    11,
			wantErr: false,
		},
		{
			name:    "Invalid negative line number",
			line:    -1,
			col:     0,
			want:    0,
			wantErr: true,
		},
		{
			name:    "Invalid negative column number",
			line:    1,
			col:     -1,
			want:    0,
			wantErr: true,
		},
		{
			name:    "Column number out of range",
			line:    1,
			col:     15,
			want:    10, // Should return the end of the line offset
			wantErr: false,
		},
		{
			name:    "Line number out of range",
			line:    10,
			col:     0,
			want:    66, // Should return the last valid offset (end of the last line)
			wantErr: true,
		},
		{
			name:    "Last valid position",
			line:    6,
			col:     10,
			want:    55,
			wantErr: false,
		},
		{
			name:    "First position of last line",
			line:    6,
			col:     0,
			want:    45,
			wantErr: false,
		},
		{
			name:    "Empty line start",
			line:    3,
			col:     0,
			want:    22, // Start offset of the line after the empty line
			wantErr: false,
		},
		{
			name:    "Empty line end",
			line:    3,
			col:     1,
			want:    22, // End offset of the empty line (same as start, because the line is empty)
			wantErr: false,
		},
		{
			name:    "Empty line end",
			line:    3,
			col:     11111,
			want:    22, // End offset of the empty line (same as start, because the line is empty)
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := editor.GetOffsetByPositionWithError(tt.line, tt.col+1)
			if (err != nil) != tt.wantErr {
				t.Errorf("%s: GetOffsetByPositionRaw() expected error = %v, got error = %v", tt.name, tt.wantErr, err)
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("%s: GetOffsetByPositionRaw() expected = %d, got = %d", tt.name, tt.want, got)
			}
		})
	}
}

func TestGetOffsetByPosition_TABLE_2(t *testing.T) {
	editor := NewMemEditor("0123456789\n0123456789\n\n0123456789\n0123456789\n0123456789\n0123456789")

	tests := []struct {
		name      string
		line, col int
		want      int
		wantErr   bool
	}{
		{
			name:    "Start of first line",
			line:    1,
			col:     0,
			want:    0,
			wantErr: false,
		},
		{
			name:    "End of first line",
			line:    1,
			col:     10,
			want:    10,
			wantErr: false,
		},
		{
			name:    "Start of second line",
			line:    2,
			col:     0,
			want:    11,
			wantErr: false,
		},
		{
			name:    "Invalid negative line number",
			line:    -1,
			col:     0,
			want:    0,
			wantErr: true,
		},
		{
			name:    "Invalid negative column number",
			line:    1,
			col:     -1,
			want:    0,
			wantErr: true,
		},
		{
			name:    "Column number out of range",
			line:    1,
			col:     15,
			want:    10, // Should return the end of the line offset
			wantErr: false,
		},
		{
			name:    "Line number out of range",
			line:    10,
			col:     0,
			want:    66, // Should return the last valid offset
			wantErr: true,
		},
		{
			name:    "Last valid position",
			line:    6,
			col:     10,
			want:    55,
			wantErr: false,
		},
		{
			name:    "First position of last line",
			line:    6,
			col:     0,
			want:    45,
			wantErr: false,
		},
		{
			name:    "End of third line (empty line)",
			line:    3,
			col:     0,
			want:    22, // Should return the start of the fourth line, as third is empty
			wantErr: false,
		},
		{
			name:    "Middle of last line",
			line:    8,
			col:     5,
			want:    66,
			wantErr: true,
		},
		{
			name:    "Column number at newline character",
			line:    2,
			col:     10,
			want:    21, // Right at the newline of second line
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := editor.GetOffsetByPositionWithError(tt.line, tt.col+1)
			if (err != nil) != tt.wantErr {
				t.Errorf("%s: GetOffsetByPositionRaw() expected error = %v, got error = %v", tt.name, tt.wantErr, err)
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("%s: GetOffsetByPositionRaw() expected = %d, got = %d", tt.name, tt.want, got)
			}
		})
	}
}

func TestGetPositionByOffset(t *testing.T) {
	editor := NewMemEditor("Hello\nWorld\nThis is a test")
	tests := []struct {
		name             string
		offset           int
		expectedLine     int
		expectedColumn   int
		expectedPosition *Position
	}{
		{
			name:           "Start of file",
			offset:         0,
			expectedLine:   1,
			expectedColumn: 0,
			expectedPosition: &Position{
				line:   1,
				column: 0,
			},
		},
		{
			name:           "End of file",
			offset:         26,
			expectedLine:   3,
			expectedColumn: 14,
			expectedPosition: &Position{
				line:   4,
				column: 14,
			},
		},
		{
			name:             "Negative offset",
			offset:           -1,
			expectedLine:     1,
			expectedColumn:   0,
			expectedPosition: &Position{line: 1, column: 0},
		},
		{
			name:             "Offset beyond EOF",
			offset:           100,
			expectedLine:     3,
			expectedColumn:   14,
			expectedPosition: &Position{line: 4, column: 14},
		},
		{
			name:           "Middle of line",
			offset:         7,
			expectedLine:   2,
			expectedColumn: 1,
			expectedPosition: &Position{
				line:   2,
				column: 1,
			},
		},
		{
			name:           "End of line",
			offset:         5,
			expectedLine:   1,
			expectedColumn: 5,
			expectedPosition: &Position{
				line:   1,
				column: 5,
			},
		},
	}

	for _, tt := range tests {
		pos := editor.GetPositionByOffset(tt.offset)
		if pos.GetLine() != tt.expectedLine || pos.GetColumn() != tt.expectedColumn+1 {
			t.Errorf("%s: GetPositionByOffset() got = (%d,%d), want = (%d,%d)",
				tt.name, pos.GetLine(), pos.GetColumn(), tt.expectedLine, tt.expectedColumn)
		}
	}
}

func TestGetPositionByOffsetWithError(t *testing.T) {
	sourceCode := "0123456789\n0123456789\n0123456789\n0123456789\n0123456789\n0123456789"
	editor := NewMemEditor(sourceCode)

	tests := []struct {
		name    string
		offset  int
		wantPos *Position
		wantErr bool
	}{
		{"Start of file", 0, NewPosition(1, 0), false},
		{"End of first line", 10, NewPosition(1, 10), false},
		{"Start of second line", 11, NewPosition(2, 0), false},
		{"Middle of third line", 25, NewPosition(3, 3), false},
		{"End of last line (before new line)", 58, NewPosition(6, 3), false},
		{"New line of last line", 52, NewPosition(5, 8), false},
		{"New line of last line2", 53, NewPosition(5, 9), false},
		{"New line of last line3", 54, NewPosition(5, 10), false},
		{"New line of last line3", 55, NewPosition(6, 0), false},
		{"New line of last line3", 56, NewPosition(6, 1), false},
		{"Just after last new line", 60, NewPosition(6, 5), false},
		{"Way out of bounds", 100, NewPosition(6, 10), true},
		{"Negative offset", -1, NewPosition(1, 0), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotPos, err := editor.GetPositionByOffsetWithError(tt.offset)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetPositionByOffsetWithError() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && !positionsEqual(gotPos, tt.wantPos) {
				t.Errorf("GetPositionByOffsetWithError() = %v, want %v", gotPos, tt.wantPos)
			}
		})
	}
}

func positionsEqual(got, want *Position) bool {
	return got.GetLine() == want.GetLine() && got.GetColumn() == want.GetColumn()+1
}
