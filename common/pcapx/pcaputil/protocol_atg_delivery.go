package pcaputil

import "math"

// The already checksum-validated computer-format i202 report contains a
// decimal tank/delivery count and ten fixed IEEE754 binary32 observations.
// Values retain unknown console units and do not establish a physical delivery.
func decodeATGDeliveryBody(w []byte, f map[string]any, limit int) (map[string]any, error) {
	selector, err := atgDecimal(w[5:7], 99)
	if err != nil {
		return nil, err
	}
	if err = atgDate(w[7:17], f); err != nil {
		return nil, err
	}
	f["Packet Name"], f["Function"], f["Tank Selector"] = "In-Tank Delivery Response", "i202", selector
	body := w[17 : len(w)-7]
	tanks := []map[string]any{}
	seen := [100]bool{}
	total := 0
	for len(body) > 0 {
		if len(tanks) >= limit {
			return nil, protocolError(ErrResourceExceeded, "ATG delivery tank count exceeds budget")
		}
		if len(body) < 5 {
			return nil, atgMalformed("delivery tank header truncated")
		}
		tank, x := atgDecimal(body[:2], 99)
		if x != nil {
			return nil, x
		}
		if tank == 0 || seen[tank] {
			return nil, atgMalformed("delivery tank number is zero or duplicate")
		}
		seen[tank] = true
		if selector != 0 && tank != selector {
			return nil, atgMalformed("selected tank differs from delivery record")
		}
		if body[2] < 32 || body[2] > 126 {
			return nil, atgMalformed("delivery product is not printableASCII")
		}
		count, x := atgDecimal(body[3:5], 99)
		if x != nil {
			return nil, x
		}
		if count > limit-total {
			return nil, protocolError(ErrResourceExceeded, "ATG retained delivery count exceeds budget")
		}
		total += count
		row := map[string]any{"Tank Number": tank, "Product Code": string(body[2:3]), "Delivery Count": count}
		body = body[5:]
		deliveries := []map[string]any{}
		for i := 0; i < count; i++ {
			if len(body) < 22 {
				return nil, atgMalformed("delivery dates/count truncated")
			}
			start, end := map[string]any{}, map[string]any{}
			if x = atgDate(body[:10], start); x != nil {
				return nil, x
			}
			if x = atgDate(body[10:20], end); x != nil {
				return nil, x
			}
			n, x := atgHex(body[20:22])
			if x != nil {
				return nil, x
			}
			if n != 10 {
				return nil, protocolError(ErrUnsupportedFeature, "ATG delivery profile requires ten values")
			}
			if limit < 10 {
				return nil, protocolError(ErrResourceExceeded, "ATG delivery value count exceeds budget")
			}
			if len(body) < 102 {
				return nil, atgMalformed("delivery float fields truncated")
			}
			d := map[string]any{"Starting Date": start, "Ending Date": end, "Value Count": 10}
			for j, k := range []string{"Starting Volume", "Starting TC Volume", "Starting Water", "Starting Temp", "Ending Volume", "Ending TC Volume", "Ending Water", "Ending Temp", "Starting Height", "Ending Height"} {
				bits, x := atgHex(body[22+8*j : 30+8*j])
				if x != nil {
					return nil, x
				}
				v := math.Float32frombits(uint32(bits))
				if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
					return nil, protocolError(ErrUnsupportedFeature, "ATG delivery non-finite IEEE754 value requires quality interpretation")
				}
				d[k] = float64(v)
			}
			deliveries = append(deliveries, d)
			body = body[102:]
		}
		row["Deliveries"] = deliveries
		tanks = append(tanks, row)
	}
	if selector != 0 && len(tanks) != 1 {
		return nil, atgMalformed("specific delivery tank response requires one record")
	}
	f["Tanks"], f["Tank Count"], f["Delivery Count"] = tanks, len(tanks), total
	return f, nil
}
