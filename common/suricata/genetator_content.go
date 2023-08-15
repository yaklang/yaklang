package suricata

import "errors"

type ContentGen struct {
	Modifiers []ByteMapModifier
	Len       int
}

func (c *ContentGen) Gen() []byte {
	bm := NewByteMap(c.Len)
	for i := 0; i < len(c.Modifiers); i++ {
		err := c.Modifiers[i].Modify(bm)
		if err != nil {
			if errors.Is(err, ErrOverFlow) {
				c.Len <<= 1
			} else {
				return nil
			}
		}
	}
	bm.FillLeftWithNoise()
	return bm.Bytes()
}
