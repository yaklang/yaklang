package jodatime

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestParse(t *testing.T) {
	tests := []struct {
		format   string
		value    string
		expected time.Time
	}{
		{"dd/MM/YYYY HH:mm:ss.SSSSSSSSS P", "03/02/2007 23:10:05.000000004 P",
			time.Date(2007, time.February, 3, 23, 10, 5, 4, time.UTC)},

		{"dd/MMMM/yyyy:HH:mm:ss Z", "30/August/2015:21:44:25 -0500",
			time.Date(2015, time.August, 30, 21, 44, 25, 0, time.FixedZone("", -5*60*60))},

		{"dd/MMMM/yyyy:hh:m:s a Z P", "30/August/2015:03:4:5 PM -0500 P",
			time.Date(2015, time.August, 30, 15, 4, 5, 0, time.FixedZone("", -5*60*60))},

		{"dd/MMMM/yyyy:hh:m:s a Z", "30/August/2015:03:4:25 PM -0500",
			time.Date(2015, time.August, 30, 15, 4, 25, 0, time.FixedZone("", -5*60*60))},

		{"YYYY-MM-dd HH:mm:ss.SSS", "2012-12-22 12:53:30.000",
			time.Date(2012, time.December, 22, 12, 53, 30, 0, time.UTC)},
		{"E d-MMMM-YY HH:mm:ss.SSS", "Mon 1-may-17 12:53:30.000",
			time.Date(2017, time.May, 1, 12, 53, 30, 0, time.UTC)},
		{"[EEE MMM dd HH:mm:ss y]", "[Sun Jan 11 10:43:35 2015]",
			time.Date(2015, time.January, 11, 10, 43, 35, 0, time.UTC)},
		{"[EEEE MMM dd HH:mm:ss y]", "[Sunday Jan 11 10:43:35 2015]",
			time.Date(2015, time.January, 11, 10, 43, 35, 0, time.UTC)},
		{"[EEEE M dd h:mm:ss y]", "[Sunday 1 11 9:43:35 2015]",
			time.Date(2015, time.January, 11, 9, 43, 35, 0, time.UTC)},

		{"dd/MMMM/yyyy:hh:m:s a ZZ", "30/August/2015:03:4:25 PM -05:00",
			time.Date(2015, time.August, 30, 15, 4, 25, 0, time.FixedZone("", -5*60*60))},

		{"YYYY-MM-dd''HH:mm:ss", "2017-02-18'16:33:21",
			time.Date(2017, time.February, 18, 16, 33, 21, 0, time.UTC)},

		{"YYYY-MM-dd'T'HH:mm:ss", "2017-02-18T16:33:21",
			time.Date(2017, time.February, 18, 16, 33, 21, 0, time.UTC)},

		{"YYYY-MM-dd'T'HH:mm:ss'Z'", "2017-02-18T16:33:21Z",
			time.Date(2017, time.February, 18, 16, 33, 21, 0, time.UTC)},

		{"YYYY-MM-dd HH:mm:ss.SSS", "2012-12-22 12:53:30.123",
			time.Date(2012, time.December, 22, 12, 53, 30, 123*1000000, time.UTC)},
		{"YYYY-MM-dd HH:mm:ss.SS", "2012-12-22 12:53:30.12",
			time.Date(2012, time.December, 22, 12, 53, 30, 12*10000000, time.UTC)},
		{"YYYY-MM-dd HH:mm:ss.S", "2012-12-22 12:53:30.1",
			time.Date(2012, time.December, 22, 12, 53, 30, 1*100000000, time.UTC)},
	}

	for _, test := range tests {
		rTime, err := Parse(test.format, test.value)
		// z, o := rTime.Zone()
		// pp.Println("rTime, zone, offset-->", rTime, z, o/3600)

		assert.NoError(t, err)
		assert.Equal(t, test.expected, rTime, "("+test.format+") they should be equal")
	}
}

func TestParseInLocation(t *testing.T) {
	tests := []struct {
		format   string
		value    string
		location string
		expected time.Time
	}{
		{"dd/MM/YYYY HH:mm:ss", "03/02/2007 23:10:05", "Europe/Paris",
			time.Date(2007, time.February, 3, 23, 10, 5, 0, time.FixedZone("CET", 3600))},
	}

	for _, test := range tests {
		rTime, err := ParseInLocation(test.format, test.value, test.location)
		assert.NoError(t, err)
		assert.Equal(t, test.expected.Format("RFC1123Z"), rTime.Format("RFC1123Z"), "("+test.format+") they should be equal")
	}
}

func TestParseInLocationDirect(t *testing.T) {
	location, _ := time.LoadLocation("Europe/Paris")
	tests := []struct {
		format   string
		value    string
		location *time.Location
		expected time.Time
	}{
		{"dd/MM/YYYY HH:mm:ss", "03/02/2007 23:10:05", location,
			time.Date(2007, time.February, 3, 23, 10, 5, 0, time.FixedZone("CET", 3600))},
	}

	for _, test := range tests {
		rTime, err := ParseInLocationDirect(test.format, test.value, test.location)
		assert.NoError(t, err)
		assert.Equal(t, test.expected.Format("RFC1123Z"), rTime.Format("RFC1123Z"), "("+test.format+") they should be equal")
	}
}

func TestParseInLocationError(t *testing.T) {
	tests := []struct {
		format   string
		value    string
		location string
		expected time.Time
	}{
		{"dd/MM/YYYY HH:mm:ss", "03/02/2007 23:10:05", "Space/Moon",
			time.Date(2007, time.February, 3, 23, 10, 5, 0, time.FixedZone("CET", 3600))},
	}

	for _, test := range tests {
		_, err := ParseInLocation(test.format, test.value, test.location)
		assert.Error(t, err)
	}
}

func BenchmarkParse(b *testing.B) {
	// run the Parse function b.N times
	for n := 0; n < b.N; n++ {
		_, _ = Parse("YYYY-MM-dd'T'HH:mm:ss", "2017-02-18T16:33:21")
	}
}

func BenchmarkParseInLocation(b *testing.B) {
	// run the Parse function b.N times
	for n := 0; n < b.N; n++ {
		_, _ = ParseInLocation("YYYY-MM-dd'T'HH:mm:ss", "2017-02-18T16:33:21", "Europe/Moscow")
	}
}

func BenchmarkParseInLocationDirect(b *testing.B) {
	location, _ := time.LoadLocation("Europe/Moscow")
	// run the Parse function b.N times
	for n := 0; n < b.N; n++ {
		_, _ = ParseInLocationDirect("YYYY-MM-dd'T'HH:mm:ss", "2017-02-18T16:33:21", location)
	}
}

func BenchmarkTimeParse(b *testing.B) {
	layout := GetLayout("YYYY-MM-dd'T'HH:mm:ss")
	// run the Parse function b.N times
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		time.Parse(layout, "2017-02-18T16:33:21")
	}
}

func BenchmarkGetLayout(b *testing.B) {
	for n := 0; n < b.N; n++ {
		_ = GetLayout(layoutLong)
	}
}
