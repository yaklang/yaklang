package comparer

import "yaklang/common/utils"

/*

 */

type Discriminator struct {
	PositiveSample           []byte
	PositiveThresholdAverage float64

	NegativeSample           []byte
	NegativeThresholdAverage float64
}

func (d *Discriminator) SetNegativeSample(raw []byte) {
	d.NegativeSample = raw
}

func (d *Discriminator) AdjustPositiveThreshold(raw []byte) error {
	np := CompareHTTPResponseRaw(d.PositiveSample, raw)
	if np > 0.8 {
		d.PositiveThresholdAverage = (d.PositiveThresholdAverage + np) / 2.0
		return nil
	}
	return utils.Error("too dynamic for positive sample(threshold calc)")
}

func (d *Discriminator) AdjustNegativeThreshold(raw []byte) error {
	np := CompareHTTPResponseRaw(d.NegativeSample, raw)
	if np > 0.8 {
		d.NegativeThresholdAverage = (d.NegativeThresholdAverage + np) / 2.0
		return nil
	}
	return utils.Error("too dynamic for negative sample(threshold calc)")
}

func (d *Discriminator) IsPositive(raw []byte) bool {
	if d.PositiveThresholdAverage <= 0 || d.PositiveThresholdAverage > 1 {
		d.PositiveThresholdAverage = 0.98
	}
	return d.IsPositiveWithThreshold(raw, d.PositiveThresholdAverage)
}

func (d *Discriminator) IsPositiveWithThreshold(raw []byte, threshold float64) bool {
	if threshold <= 0 || threshold > 1 {
		threshold = 0.98
	}
	if CompareHTTPResponseRaw(d.PositiveSample, raw) > threshold {
		return true
	}
	return false
}
func (d *Discriminator) IsNegative(raw []byte) bool {
	if d.NegativeThresholdAverage <= 0 || d.NegativeThresholdAverage > 1 {
		d.NegativeThresholdAverage = 0.98
	}
	return d.IsNegativeWithThreshold(raw, d.NegativeThresholdAverage)
}

func (d *Discriminator) IsNegativeWithThreshold(raw []byte, threshold float64) bool {
	if threshold <= 0 || threshold > 1 {
		threshold = 0.98
	}
	if CompareHTTPResponseRaw(d.NegativeSample, raw) > threshold {
		return true
	}
	return false
}

func NewDiscriminator(origin []byte) *Discriminator {
	d := &Discriminator{
		PositiveSample:           origin,
		PositiveThresholdAverage: 0.98,
		NegativeThresholdAverage: 0.98,
	}
	return d
}
