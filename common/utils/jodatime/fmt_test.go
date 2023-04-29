package jodatime

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

var date, _ = time.Parse("2006-01-02 15:04:05.9999999 MST", "2007-02-03 16:05:06.1234567 UTC")

func TestFormatMoreQuotes(t *testing.T) {
	tests := []struct {
		format   string
		expected string
	}{

		{"HHTmm", "16T05"},
		{"HH''mm", "16'05"},
		{"HH''mm", "16'05"},
		{"HH'H'mm", "16H05"},
		{"HH'H''mm", "16Hmm"},
		{"HH'H'''mm", "16H'05"},
		{"HH'''H'mm", "16'H05"},
		{"HH'''H'''mm", "16'H'05"},

		{"HH'H D'mm", "16H D05"},
		{"HH'H D''mm", "16H Dmm"},
		{"HH'H D'''mm", "16H D'05"},
		{"HH'''H D'mm", "16'H D05"},
		{"HH'''H D'''mm", "16'H D'05"},

		{"'With Emoji' HH😀mm", "With Emoji 16😀05"},
		{"'With Emoji' HH'😀'mm", "With Emoji 16😀05"},
		{"'With Emoji' HH'😀🕐'mm", "With Emoji 16😀🕐05"},

		{"HH 'hours and' mm 'minutes'", "16 hours and 05 minutes"},
	}

	for _, test := range tests {
		assert.Equal(t, test.expected, Format(test.format, date), "("+test.format+") they should be equal")
	}
}

func TestFormatMore(t *testing.T) {
	tests := []struct {
		format   string
		expected string
		date     time.Time
	}{
		{"a", "AM",
			time.Date(2007, time.February, 3, 11, 10, 5, 4, time.UTC)},
		{"dd/MM/YYYY HH:mm:ss ZZ", "03/02/2007 16:05:06 -01:00",
			time.Date(2007, time.February, 3, 16, 05, 6, 0, time.FixedZone("", -1*3600))},
		{"yyyy-MM-dd'T'HH:mm:ss.SSSZ", "2007-02-03T16:05:06.000-0100",
			time.Date(2007, time.February, 3, 16, 05, 6, 0, time.FixedZone("", -1*3600))},
		// ZeroFixes
		{"DD", "05", time.Date(2007, time.January, 5, 16, 05, 6, 0, time.FixedZone("", -1*3600))},
		{"ww", "14", time.Date(2007, time.April, 5, 16, 05, 6, 0, time.FixedZone("", -1*3600))},
		{"dd", "15", time.Date(2007, time.April, 15, 16, 05, 6, 0, time.FixedZone("", -1*3600))},
		{"MM", "11", time.Date(2007, time.November, 15, 16, 05, 6, 0, time.FixedZone("", -1*3600))},
		{"ee", "04", time.Date(2007, time.November, 15, 16, 05, 6, 0, time.FixedZone("", -1*3600))},
		{"hh", "11", time.Date(2007, time.November, 15, 11, 05, 6, 0, time.FixedZone("", -1*3600))},
		{"hh", "08", time.Date(2007, time.November, 15, 20, 05, 6, 0, time.FixedZone("", -1*3600))},
		{"HH", "04", time.Date(2007, time.November, 15, 04, 05, 6, 0, time.FixedZone("", -1*3600))},
		{"mm", "05", time.Date(2007, time.November, 15, 04, 05, 6, 0, time.FixedZone("", -1*3600))},
		{"mm", "55", time.Date(2007, time.November, 15, 04, 55, 6, 0, time.FixedZone("", -1*3600))},
		{"ss", "46", time.Date(2007, time.November, 15, 04, 55, 46, 0, time.FixedZone("", -1*3600))},

		{"SS", "00", time.Date(2007, time.November, 15, 04, 55, 46, 9, time.FixedZone("", -1*3600))},
		{"SSS", "000", time.Date(2007, time.November, 15, 04, 55, 46, 9, time.FixedZone("", -1*3600))},
		{"SSS", "100", time.Date(2007, time.November, 15, 04, 55, 46, 100000000, time.FixedZone("", -1*3600))},
		{"SSS", "010", time.Date(2007, time.November, 15, 04, 55, 46, 10000000, time.FixedZone("", -1*3600))},
		{"SSS", "001", time.Date(2007, time.November, 15, 04, 55, 46, 1000000, time.FixedZone("", -1*3600))},

		{"Z", "-1100", time.Date(2007, time.November, 15, 04, 55, 46, 1000000, time.FixedZone("", -11*3600))},
		{"ZZ", "-11:00", time.Date(2007, time.November, 15, 04, 55, 46, 1000000, time.FixedZone("", -11*3600))},
		{"KK", "11", time.Date(2007, time.November, 15, 23, 55, 46, 1000000, time.FixedZone("", -11*3600))},
		{"kk", "08", time.Date(2007, time.November, 15, 8, 55, 46, 1000000, time.FixedZone("", -11*3600))},
	}

	for _, test := range tests {
		assert.Equal(t, test.expected, Format(test.format, test.date), "("+test.format+") they should be equal")
	}

}
func TestFormat(t *testing.T) {

	tests := []struct {
		format   string
		expected string
	}{

		{"Y", "2007"},
		{"YY", "07"},
		{"YYY", "2007"},
		{"YYYY", "2007"},
		{"y", "2007"},
		{"yy", "07"},
		{"yyy", "2007"},
		{"yyyy", "2007"},

		{"D ", "34 "},
		{"DD", "34"},

		{"M", "2"},
		{"MM", "02"},
		{"MMM", "Feb"},
		{"MMMM", "February"},

		{"d", "3"},
		{"dd", "03"},

		{"e", "6"},
		{"ee", "06"},
		{"eeL", "06L"},

		{"E", "Sat"},
		{"EE", "Sat"},
		{"EEE", "Sat"},
		{"EEEE", "Saturday"},

		{"h", "4"},
		{"hh", "04"},

		{"H", "16"},
		{"HH", "16"},

		{"a", "PM"},

		{"m", "5"},
		{"mm", "05"},

		{"s", "6"},
		{"ss", "06"},

		{"S", "1"},
		{"SS", "12"},
		{"SSSL", "123L"},

		{"zL", "UTCL"},

		{"ZL", "+0000L"},
		{"ZZ", "+00:00"},
		{"ZZZ", ""},

		{"K", "4"},
		{"KKL", "04L"},

		{"kL", "16L"},
		{"kk k", "16 16"},

		{"w", "5"},
		{"ww", "05"},
		{"wwT", "05T"},

		{"YYYY.MM.dd", "2007.02.03"},
		{"YYYY.MM.d", "2007.02.3"},
		{"YYYY.M.d", "2007.2.3"},
		{"YY.M.d", "07.2.3"},

		{"dd MMM YYYY", "03 Feb 2007"},
		{"dd MMMM YYYY", "03 February 2007"},

		{"E dd MMMM YYYY", "Sat 03 February 2007"},
		{"EE dd MMMM YYYY", "Sat 03 February 2007"},
		{"EEE dd MMMM YYYY", "Sat 03 February 2007"},
		{"EEEE dd MMMM YYYY", "Saturday 03 February 2007"},

		{"HH:mm:ss", "16:05:06"},
		{"HH:mm:s", "16:05:6"},
		{"HH:m:s", "16:5:6"},
		{"H:m:s", "16:5:6"},

		{"hh:m:s", "04:5:6"},
		{"h:m:s", "4:5:6"},

		{"HH:mm:ss.SSS", "16:05:06.123"},
		{"HH:mm:ss.SS", "16:05:06.12"},
		{"HH:mm:ss.S", "16:05:06.1"},

		{"dd/MM/YYYY HH:mm:ss z", "03/02/2007 16:05:06 UTC"},

		{"dd/MM/YYYY HH:mm:ss z ZZ", "03/02/2007 16:05:06 UTC +00:00"},
		{"dd/MM/YYYY HH:mm:ss a 世 z Z", "03/02/2007 16:05:06 PM 世 UTC +0000"},
	}

	for _, test := range tests {
		assert.Equal(t, test.expected, Format(test.format, date), "("+test.format+") they should be equal")
	}

}

func TestFormatEra(t *testing.T) {
	check(t, "G", "AD", "1945-01-02T15:04:05+07:00")
	check(t, "G", "AD", "2007-01-31T15:04:05Z")
}

func TestFormatCenturyOfEra(t *testing.T) {
	check(t, "C", "20", "2006-01-02T15:04:05+07:00")
	check(t, "C", "20", "2007-01-31T15:04:05Z")
	check(t, "C", "20", "2006-01-02T15:04:05+07:00")
	check(t, "C", "19", "1999-01-02T15:04:05+07:00")
	check(t, "C", "17", "1789-01-02T15:04:05+07:00")
}

func TestFormatYearOfEra(t *testing.T) {
	check(t, "Y", "2004", "2004-01-02T15:04:05+07:00")
	check(t, "Y", "1945", "1945-01-31T15:04:05Z")

	check(t, "YY", "04", "2004-01-02T15:04:05+07:00")
	check(t, "YY", "45", "1945-01-31T15:04:05Z")

	check(t, "YYY", "1945", "1945-01-31T15:04:05Z")
	check(t, "YYYY", "1945", "1945-01-31T15:04:05Z")
}

func TestFormatYear(t *testing.T) {
	check(t, "y", "2004", "2004-01-02T15:04:05+07:00")
	check(t, "y", "1945", "1945-01-31T15:04:05Z")

	check(t, "yy", "04", "2004-01-02T15:04:05+07:00")
	check(t, "yy", "45", "1945-01-31T15:04:05Z")

	check(t, "yyy", "1945", "1945-01-31T15:04:05Z")
	check(t, "yyyy", "1945", "1945-01-31T15:04:05Z")
}

func TestFormatWeekYear(t *testing.T) {
	check(t, "x", "2004", "2004-01-02T15:04:05+07:00")
	check(t, "x", "1945", "1945-01-31T15:04:05Z")

	check(t, "xx", "04", "2004-01-02T15:04:05+07:00")
	check(t, "xx", "45", "1945-01-31T15:04:05Z")

	check(t, "xxx", "1945", "1945-01-31T15:04:05Z")
	check(t, "xxxx", "1945", "1945-01-31T15:04:05Z")
}

func TestFormatWeekOfWeekyear(t *testing.T) {
	check(t, "w", "24", "2004-06-10T15:04:05+07:00")
	check(t, "w", "5", "1945-01-31T15:04:05Z")

	check(t, "ww", "24", "2004-06-10T15:04:05+07:00")
	check(t, "ww", "01", "2004-01-02T15:04:05+07:00")
	check(t, "ww", "05", "1945-01-31T15:04:05Z")

	check(t, "www", "", "1945-01-31T15:04:05Z")
	check(t, "wwww", "", "1945-01-31T15:04:05Z")
}

func TestFormatHourOfHalfday(t *testing.T) { //0~11
	check(t, "K", "0", "2004-06-09T00:20:05+05:00")
	check(t, "K", "10", "2004-06-09T10:20:05+07:00")
	check(t, "K", "0", "2004-06-09T12:20:05+05:00")
	check(t, "K", "10", "2004-06-09T22:20:05+00:00")

	check(t, "KK", "00", "2004-06-09T00:20:05+05:00")
	check(t, "KK", "10", "2004-06-09T10:20:05+07:00")
	check(t, "KK", "00", "2004-06-09T12:20:05+05:00")
	check(t, "KK", "10", "2004-06-09T22:20:05+00:00")
}

func TestFormatClockhourOfHalfday(t *testing.T) { // clockhour of halfday (1~12)
	check(t, "h", "12", "2004-06-09T00:20:05+05:00")
	check(t, "h", "10", "2004-06-09T10:20:05+07:00")
	check(t, "h", "4", "2004-06-09T4:20:05+07:00")
	check(t, "h", "12", "2004-06-09T12:20:05+05:00")
	check(t, "h", "10", "2004-06-09T22:20:05+00:00")
	check(t, "h", "11", "2004-06-09T23:20:05+00:00")

	check(t, "hh", "12", "2004-06-09T00:20:05+05:00")
	check(t, "hh", "04", "2004-06-09T04:20:05+07:00")
	check(t, "hh", "12", "2004-06-09T12:20:05+05:00")
	check(t, "hh", "10", "2004-06-09T22:20:05+00:00")
	check(t, "hh", "11", "2004-06-09T23:20:05+00:00")
}

func TestFormatHourOfDay(t *testing.T) { // clockhour of halfday (1~12)
	check(t, "H", "0", "2004-06-09T00:20:05+05:00")
	check(t, "H", "4", "2004-06-09T04:20:05+07:00")
	check(t, "H", "10", "2004-06-09T10:20:05+07:00")
	check(t, "H", "12", "2004-06-09T12:20:05+05:00")
	check(t, "H", "22", "2004-06-09T22:20:05+00:00")
	check(t, "H", "23", "2004-06-09T23:20:05+00:00")

	check(t, "HH", "00", "2004-06-09T00:20:05+05:00")
	check(t, "HH", "04", "2004-06-09T04:20:05+07:00")
	check(t, "HH", "10", "2004-06-09T10:20:05+07:00")
	check(t, "HH", "12", "2004-06-09T12:20:05+05:00")
	check(t, "HH", "22", "2004-06-09T22:20:05+00:00")
	check(t, "HH", "23", "2004-06-09T23:20:05+00:00")
}

func TestFormatClockhourOfDay(t *testing.T) { // clockhour of halfday (1~12)
	check(t, "k", "24", "2004-06-09T00:20:05+05:00")
	check(t, "k", "1", "2004-06-09T1:20:05+07:00")
	check(t, "k", "4", "2004-06-09T4:20:05+07:00")
	check(t, "k", "10", "2004-06-09T10:20:05+07:00")
	check(t, "k", "12", "2004-06-09T12:20:05+05:00")
	check(t, "k", "22", "2004-06-09T22:20:05+00:00")
	check(t, "k", "23", "2004-06-09T23:20:05+00:00")

	check(t, "kk", "24", "2004-06-09T00:20:05+05:00")
	check(t, "kk", "01", "2004-06-09T01:20:05+07:00")
	check(t, "kk", "04", "2004-06-09T04:20:05+07:00")
	check(t, "kk", "12", "2004-06-09T12:20:05+05:00")
	check(t, "kk", "22", "2004-06-09T22:20:05+00:00")
	check(t, "kk", "23", "2004-06-09T23:20:05+00:00")
}

// RFC3339     = "2006-01-02T15:04:05Z07:00"
func check(t *testing.T, format, expected, date string) {
	if mt, err := time.Parse(time.RFC3339, date); err == nil {
		assert.Equal(t, expected, Format(format, mt), fmt.Sprintf("%s/ pattern '%s', with '%s'", t.Name(), format, date))
	} else {
		t.Errorf("date parse error - %s", err)

	}
}

var (
	layoutShort = "d/M/YY h:m:s z"
	layoutLong  = "dd/MM/YYYY HH:mm:ss z"
)

func BenchmarkFormat(b *testing.B) {
	// run the Parse function b.N times
	for n := 0; n < b.N; n++ {
		Format(layoutLong, date)
	}
}

func BenchmarkTimeFormat(b *testing.B) {
	layout := GetLayout(layoutLong)
	// run the Parse function b.N times
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		date.Format(layout)
	}
}

func BenchmarkFormatShort(b *testing.B) {
	// run the Parse function b.N times
	for n := 0; n < b.N; n++ {
		Format(layoutShort, date)
	}
}

func BenchmarkTimeFormatShort(b *testing.B) {
	layout := GetLayout(layoutShort)
	// run the Parse function b.N times
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		date.Format(layout)
	}
}
