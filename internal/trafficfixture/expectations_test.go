package trafficfixture

import (
	"encoding/json"
	"sync"
	"testing"
)

func TestAllExpectationsOwnedConcurrent(t *testing.T) {
	baseline, err := AllExpectations()
	if err != nil {
		t.Fatal(err)
	}
	before, err := json.Marshal(baseline)
	if err != nil {
		t.Fatal(err)
	}
	mutate := func(rows []*FrozenExpectations) {
		for _, row := range rows {
			row.SourceSHA = "caller-mutated"
			for i := range row.Cases {
				c := &row.Cases[i]
				c.ID = "caller-mutated"
				if len(c.Missing) > 0 {
					c.Missing[0] = "caller-mutated"
				}
				if c.Facts.Representative != nil {
					c.Facts.Representative.Number = 999999
				}
				for j := range c.Expectations {
					e := &c.Expectations[j]
					e.Protocol = "caller-mutated"
					if e.Frame != nil {
						e.Frame.Number = 999999
					}
					if len(e.Selection) > 0 {
						e.Selection[0] = '?'
					}
					if len(e.PayloadConstraints) > 0 {
						e.PayloadConstraints[0] = '?'
					}
					if len(e.Expected.RequiredFields) > 0 {
						e.Expected.RequiredFields[0] = "caller-mutated"
					}
					for key, value := range e.Expected.Values {
						if len(value) > 0 {
							value[0] = '?'
						}
						delete(e.Expected.Values, key)
					}
					if e.Expected.EventCounts != nil {
						e.Expected.EventCounts["caller-mutated"] = 999999
					}
				}
			}
		}
	}
	c, err := load()
	if err != nil {
		t.Fatal(err)
	}
	original, err := readAllExpectations(c)
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	if string(canonical) != string(before) {
		t.Fatal("cached public projection changed original inventory semantics, including nil/empty fields")
	}
	mutate(baseline)
	var wg sync.WaitGroup
	for worker := 0; worker < 4; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rows, err := AllExpectations()
			if err != nil {
				t.Error(err)
				return
			}
			got, err := json.Marshal(rows)
			if err != nil {
				t.Error(err)
				return
			}
			if string(got) != string(before) {
				t.Error("another caller changed the frozen answer inventory")
			}
			mutate(rows)
		}()
	}
	wg.Wait()
	after, err := AllExpectations()
	if err != nil {
		t.Fatal(err)
	}
	got, err := json.Marshal(after)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(before) {
		t.Fatal("nested mutation escaped into future answer queries")
	}
}
