package comparer

import "github.com/yaklang/yaklang/common/utils"

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

// NewDiscriminator 基于一个原始响应样本创建一个判别器，用于判断其它响应是否与样本相似
// 参数:
//   - origin: 作为正样本的原始 HTTP 响应报文
//
// 返回值:
//   - 判别器对象，可调用 IsNegative 等方法进行相似度判别
//
// Example:
// ```
// // VARS: 基于样本创建判别器
// d = judge.NewDiscriminator("HTTP/1.1 200 OK\r\nContent-Length: 5\r\n\r\nhello")
// // assert: 与负样本(空)相比不相似，IsNegative 返回 false
// assert d.IsNegative("HTTP/1.1 200 OK\r\nContent-Length: 5\r\n\r\nhello") == false, "discriminator should be constructed and callable"
// ```
func NewDiscriminator(origin []byte) *Discriminator {
	d := &Discriminator{
		PositiveSample:           origin,
		PositiveThresholdAverage: 0.98,
		NegativeThresholdAverage: 0.98,
	}
	return d
}
