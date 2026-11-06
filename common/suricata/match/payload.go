package match

import (
	"bytes"
	"encoding/base64"
	"math/bits"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	"github.com/yaklang/yaklang/common/suricata/data"
	"github.com/yaklang/yaklang/common/suricata/data/modifier"
	"github.com/yaklang/yaklang/common/suricata/data/numrange"
	"github.com/yaklang/yaklang/common/suricata/rule"
	"golang.org/x/exp/slices"
)

func newPayloadMatcher(r *rule.ContentRule, mdf modifier.Modifier) (matchHandler, error) {
	var bsizeRange, dsizeRange *numrange.NumRange
	var err error
	if r.BSize != "" {
		bsizeRange, err = numrange.ParseNumRange(r.BSize)
		if err != nil {
			return nil, errors.Wrap(err, "invalid bsize")
		}
	}
	if r.DSize != "" {
		dsizeRange, err = numrange.ParseNumRange(r.DSize)
		if err != nil {
			return nil, errors.Wrap(err, "invalid dsize")
		}
	}
	if r.DotPrefix && mdf == modifier.Default {
		return nil, errors.New("dotprefix requires a sticky buffer")
	}

	var contentMatcher matchHandler
	if r.PCREParsed != nil {
		contentMatcher, err = newPCREMatch(r)
		if err != nil {
			return nil, err
		}
	}
	byteOps, err := compileByteOps(r)
	if err != nil {
		return nil, err
	}
	base64Spec, err := compileBase64Decode(r.Base64Decode)
	if err != nil {
		return nil, err
	}

	return func(c *matchContext) (handlerErr error) {
		buffer := c.GetBuffer(mdf)
		if r.RawBytes && c.PK != nil {
			buffer = c.PK.Data()
		}
		if r.DotPrefix {
			buffer = dotPrefixBuffer(buffer)
		}
		if len(byteOps) > 0 || base64Spec != nil {
			defer func() {
				if handlerErr != nil || c.IsRejected() {
					return
				}
				if len(byteOps) > 0 {
					if err := matchByteOps(c, mdf, buffer, byteOps); err != nil {
						handlerErr = err
						return
					}
					if c.IsRejected() {
						return
					}
				}
				if base64Spec != nil {
					handlerErr = c.applyBase64Decode(base64Spec, mdf, buffer)
				}
			}()
		}
		if bsizeRange != nil && !c.Must(bsizeRange.Match(len(buffer))) {
			return nil
		}
		if dsizeRange != nil {
			packetPayload := c.GetBuffer(modifier.Default)
			if transportLayer := c.PK.TransportLayer(); transportLayer != nil {
				packetPayload = transportLayer.LayerPayload()
			}
			if !c.Must(dsizeRange.Match(len(packetPayload))) {
				return nil
			}
		}
		if contentMatcher != nil {
			return contentMatcher(c)
		}
		if len(r.Content) == 0 {
			if r.IsDataAt == "" {
				return nil
			}
			return matchStandaloneIsDataAt(c, buffer, r.IsDataAt)
		}

		offset, depth, distance, within, ok := resolveContentBounds(c, r)
		if !ok {
			c.Reject()
			return nil
		}
		jumpPosition, hasJump := c.takeJumpCursor(mdf)
		indexes := bytesIndexAll(buffer, r.Content, r.Nocase)
		if hasJump && offset == nil && distance == nil {
			indexes = slices.DeleteFunc(indexes, func(match data.Matched) bool {
				return match.Pos < jumpPosition
			})
		}

		defer func() {
			if r.Negative && c.IsRejected() {
				c.Recover()
			} else if r.Negative && !c.IsRejected() {
				c.Reject()
			}
		}()

		if !c.Must(len(indexes) > 0) {
			return nil
		}

		if r.StartsWith {
			expectedStart := 0
			if hasJump && offset == nil && distance == nil {
				expectedStart = jumpPosition
			}
			if !c.Must(indexes[0].Pos == expectedStart) {
				return nil
			}
			c.SetPrevMatched(mdf, []data.Matched{indexes[0]})
			return nil
		}

		if r.EndsWith {
			targetPos := len(buffer) - len(r.Content)
			if depth != nil {
				targetPos = *depth - len(r.Content)
			}

			idx, found := slices.BinarySearchFunc(indexes, targetPos, func(m data.Matched, i int) int {
				return m.Pos - i
			})
			if !c.Must(found) {
				return nil
			}
			c.SetPrevMatched(mdf, []data.Matched{indexes[idx]})
			return nil
		}

		if depth != nil || offset != nil {
			left := 0
			if offset != nil {
				left = *offset
			}
			if !c.Must(left >= 0) {
				return nil
			}
			indexes = slices.DeleteFunc(indexes, func(m data.Matched) bool {
				if m.Pos < left {
					return true
				}
				return depth != nil && (*depth < 0 || m.Pos+m.Len > left+*depth)
			})
			if !c.Must(len(indexes) != 0) {
				return nil
			}
		}

		prevMatch, existed := c.GetPrevMatched(mdf)

		if distance != nil && existed {
			indexes = slices.DeleteFunc(indexes, func(m data.Matched) bool {
				for _, pm := range prevMatch {
					if m.Pos >= pm.Pos+pm.Len+*distance {
						return false
					}
				}
				return true
			})
			if !c.Must(len(indexes) != 0) {
				return nil
			}
		}

		if within != nil && existed {
			indexes = slices.DeleteFunc(indexes, func(m data.Matched) bool {
				for _, pm := range prevMatch {
					if m.Pos >= pm.Pos+pm.Len && m.Pos+m.Len <= pm.Pos+pm.Len+*within {
						return false
					}
				}
				return true
			})
			if !c.Must(len(indexes) != 0) {
				return nil
			}
		}

		if r.IsDataAt != "" {
			strpos := strings.Split(r.IsDataAt, ",")
			var neg bool
			var strnum string
			if len(strpos[0]) > 1 && strpos[0][0] == '!' {
				neg = true
				strnum = strpos[0][1:]
			} else {
				strnum = strpos[0]
			}
			pos, err := strconv.Atoi(strnum)
			if err != nil {
				return errors.Wrap(err, "isdataat format error")
			}
			if len(strpos) == 1 {
				indexes = slices.DeleteFunc(indexes, func(data.Matched) bool {
					return negIf(neg, pos >= len(buffer))
				})
			} else {
				if !c.Must(len(strpos) == 2 && strpos[1] == "relative") {
					return errors.New("isdataat format error")
				}
				indexes = slices.DeleteFunc(indexes, func(m data.Matched) bool {
					return negIf(neg, m.Pos+m.Len+pos > len(buffer))
				})
			}
			if !c.Must(len(indexes) != 0) {
				return nil
			}
		}

		c.SetPrevMatched(mdf, indexes)
		return nil
	}, nil
}

type byteTestSpec struct {
	bytes      int
	bytesVar   string
	operator   string
	value      uint64
	valueVar   string
	offset     int
	relative   bool
	little     bool
	stringMode bool
	base       int
	bitmask    uint64
	hasBitmask bool
	shift      uint
	negate     bool
}

type byteJumpSpec struct {
	bytes         int
	bytesVar      string
	offset        int
	relative      bool
	multiplier    int
	little        bool
	stringMode    bool
	base          int
	align         bool
	fromBeginning bool
	fromEnd       bool
	postOffset    int
	bitmask       uint64
	hasBitmask    bool
}

type byteExtractSpec struct {
	bytes      int
	offset     int
	name       string
	relative   bool
	multiplier int
	little     bool
	stringMode bool
	base       int
	align      bool
	bitmask    uint64
	hasBitmask bool
}

type byteOpSpec struct {
	kind    string
	test    byteTestSpec
	jump    byteJumpSpec
	extract byteExtractSpec
}

func isByteVariableName(value string) bool {
	if value == "" {
		return false
	}
	for index, ch := range value {
		if index == 0 {
			if !(ch == '_' || ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z') {
				return false
			}
			continue
		}
		if !(ch == '_' || ch == '.' || ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch >= '0' && ch <= '9') {
			return false
		}
	}
	return true
}

func resolveContentBounds(c *matchContext, r *rule.ContentRule) (*int, *int, *int, *int, bool) {
	offset, depth, distance, within := r.Offset, r.Depth, r.Distance, r.Within
	maxInt := int64(^uint(0) >> 1)
	resolve := func(name string) (*int, bool) {
		if name == "" {
			return nil, true
		}
		value, ok := c.variables[name]
		if !ok || value < 0 || value > maxInt {
			return nil, false
		}
		resolved := int(value)
		return &resolved, true
	}
	if r.OffsetVar != "" {
		value, ok := resolve(r.OffsetVar)
		if !ok {
			return nil, nil, nil, nil, false
		}
		offset = value
	}
	if r.DepthVar != "" {
		value, ok := resolve(r.DepthVar)
		if !ok {
			return nil, nil, nil, nil, false
		}
		depth = value
	}
	if r.DistanceVar != "" {
		value, ok := resolve(r.DistanceVar)
		if !ok {
			return nil, nil, nil, nil, false
		}
		distance = value
	}
	if r.WithinVar != "" {
		value, ok := resolve(r.WithinVar)
		if !ok {
			return nil, nil, nil, nil, false
		}
		within = value
	}
	return offset, depth, distance, within, true
}

func compileByteOps(r *rule.ContentRule) ([]byteOpSpec, error) {
	rawOps := r.ByteOps
	if len(rawOps) == 0 {
		for _, raw := range r.ByteTests {
			rawOps = append(rawOps, rule.ByteOp{Kind: "test", Raw: raw})
		}
		if len(r.ByteTests) == 0 && r.ByteTest != "" {
			rawOps = append(rawOps, rule.ByteOp{Kind: "test", Raw: r.ByteTest})
		}
		for _, raw := range r.ByteJumps {
			rawOps = append(rawOps, rule.ByteOp{Kind: "jump", Raw: raw})
		}
		if len(r.ByteJumps) == 0 && r.ByteJump != "" {
			rawOps = append(rawOps, rule.ByteOp{Kind: "jump", Raw: r.ByteJump})
		}
		if r.ByteExtract != "" {
			rawOps = append(rawOps, rule.ByteOp{Kind: "extract", Raw: r.ByteExtract})
		}
	}

	specs := make([]byteOpSpec, 0, len(rawOps))
	for _, rawOp := range rawOps {
		switch rawOp.Kind {
		case "test":
			spec, err := parseByteTest(rawOp.Raw)
			if err != nil {
				return nil, errors.Errorf("byte_test %q: %v", rawOp.Raw, err)
			}
			specs = append(specs, byteOpSpec{kind: "test", test: spec})
		case "jump":
			spec, err := parseByteJump(rawOp.Raw)
			if err != nil {
				return nil, errors.Errorf("byte_jump %q: %v", rawOp.Raw, err)
			}
			specs = append(specs, byteOpSpec{kind: "jump", jump: spec})
		case "extract":
			spec, err := parseByteExtract(rawOp.Raw)
			if err != nil {
				return nil, errors.Errorf("byte_extract %q: %v", rawOp.Raw, err)
			}
			specs = append(specs, byteOpSpec{kind: "extract", extract: spec})
		default:
			return nil, errors.Errorf("unsupported byte operation %q", rawOp.Kind)
		}
	}
	return specs, nil
}

func parseByteTest(raw string) (byteTestSpec, error) {
	parts := strings.Split(raw, ",")
	if len(parts) < 4 {
		return byteTestSpec{}, errors.New("requires bytes, operator, value and offset")
	}
	for index := range parts {
		parts[index] = strings.TrimSpace(parts[index])
	}

	spec := byteTestSpec{base: 10}
	byteCount, err := strconv.Atoi(parts[0])
	if err != nil {
		if !isByteVariableName(parts[0]) {
			return byteTestSpec{}, errors.Errorf("byte count %q must be numeric or a variable", parts[0])
		}
		spec.bytesVar = parts[0]
	} else {
		if byteCount < 0 || byteCount > 8 {
			return byteTestSpec{}, errors.Errorf("byte count %d must be between 0 and 8", byteCount)
		}
		spec.bytes = byteCount
	}

	operator := parts[1]
	if strings.HasPrefix(operator, "!") {
		spec.negate = true
		operator = strings.TrimSpace(strings.TrimPrefix(operator, "!"))
		if operator == "" {
			operator = "="
		}
	}
	switch operator {
	case "=", "!=", "<", ">", "<=", ">=", "&", "^":
	default:
		return byteTestSpec{}, errors.Errorf("unsupported operator %q", parts[1])
	}
	spec.operator = operator

	value, err := strconv.ParseUint(parts[2], 0, 64)
	if err != nil {
		if !isByteVariableName(parts[2]) {
			return byteTestSpec{}, errors.Errorf("value %q must be numeric or a variable", parts[2])
		}
		spec.valueVar = parts[2]
	} else {
		spec.value = value
	}
	spec.offset, err = strconv.Atoi(parts[3])
	if err != nil {
		return byteTestSpec{}, errors.Errorf("offset %q must be numeric", parts[3])
	}

	for index := 4; index < len(parts); index++ {
		option := parts[index]
		switch strings.ToLower(option) {
		case "relative":
			spec.relative = true
		case "big":
			spec.little = false
		case "little":
			spec.little = true
		case "string":
			spec.stringMode = true
		case "dec":
			spec.base = 10
		case "hex":
			spec.base = 16
		case "oct":
			spec.base = 8
		case "dce":
			return byteTestSpec{}, errors.New("dce byte order is not implemented")
		default:
			if strings.HasPrefix(strings.ToLower(option), "bitmask ") {
				mask := strings.TrimSpace(option[len("bitmask "):])
				spec.bitmask, err = strconv.ParseUint(mask, 0, 64)
				if err != nil {
					return byteTestSpec{}, errors.Errorf("bitmask %q must be numeric", mask)
				}
				spec.hasBitmask = true
				spec.shift = uint(bits.TrailingZeros64(spec.bitmask))
				continue
			}
			return byteTestSpec{}, errors.Errorf("unsupported option %q", option)
		}
	}
	if spec.bytes == 0 && spec.bytesVar == "" && !spec.stringMode {
		return byteTestSpec{}, errors.New("zero byte count requires string mode")
	}
	return spec, nil
}

func parseByteJump(raw string) (byteJumpSpec, error) {
	parts := strings.Split(raw, ",")
	if len(parts) < 2 {
		return byteJumpSpec{}, errors.New("requires bytes and offset")
	}
	for index := range parts {
		parts[index] = strings.TrimSpace(parts[index])
	}

	spec := byteJumpSpec{multiplier: 1, base: 10}
	byteCount, err := strconv.Atoi(parts[0])
	if err != nil {
		if !isByteVariableName(parts[0]) {
			return byteJumpSpec{}, errors.Errorf("byte count %q must be numeric or a variable", parts[0])
		}
		spec.bytesVar = parts[0]
	} else {
		if byteCount < 0 || byteCount > 8 {
			return byteJumpSpec{}, errors.Errorf("byte count %d must be between 0 and 8", byteCount)
		}
		spec.bytes = byteCount
	}
	spec.offset, err = strconv.Atoi(parts[1])
	if err != nil {
		return byteJumpSpec{}, errors.Errorf("offset %q must be numeric", parts[1])
	}

	for index := 2; index < len(parts); index++ {
		option := parts[index]
		lower := strings.ToLower(option)
		switch lower {
		case "relative":
			spec.relative = true
		case "big":
			spec.little = false
		case "little":
			spec.little = true
		case "string":
			spec.stringMode = true
		case "dec":
			spec.base = 10
		case "hex":
			spec.base = 16
		case "oct":
			spec.base = 8
		case "align":
			spec.align = true
		case "from_beginning":
			spec.fromBeginning = true
		case "from_end":
			spec.fromEnd = true
		case "dce":
			return byteJumpSpec{}, errors.New("dce byte order is not implemented")
		default:
			switch {
			case strings.HasPrefix(lower, "multiplier "):
				value := strings.TrimSpace(option[len("multiplier "):])
				spec.multiplier, err = strconv.Atoi(value)
				if err != nil || spec.multiplier < 0 {
					return byteJumpSpec{}, errors.Errorf("multiplier %q must be a non-negative integer", value)
				}
			case strings.HasPrefix(lower, "post_offset "):
				value := strings.TrimSpace(option[len("post_offset "):])
				spec.postOffset, err = strconv.Atoi(value)
				if err != nil {
					return byteJumpSpec{}, errors.Errorf("post_offset %q must be numeric", value)
				}
			case strings.HasPrefix(lower, "bitmask "):
				value := strings.TrimSpace(option[len("bitmask "):])
				spec.bitmask, err = strconv.ParseUint(value, 0, 64)
				if err != nil {
					return byteJumpSpec{}, errors.Errorf("bitmask %q must be numeric", value)
				}
				spec.hasBitmask = true
			case lower == "multiplier" && index+1 < len(parts):
				index++
				spec.multiplier, err = strconv.Atoi(parts[index])
				if err != nil || spec.multiplier < 0 {
					return byteJumpSpec{}, errors.Errorf("multiplier %q must be a non-negative integer", parts[index])
				}
			case lower == "post_offset" && index+1 < len(parts):
				index++
				spec.postOffset, err = strconv.Atoi(parts[index])
				if err != nil {
					return byteJumpSpec{}, errors.Errorf("post_offset %q must be numeric", parts[index])
				}
			case lower == "bitmask" && index+1 < len(parts):
				index++
				spec.bitmask, err = strconv.ParseUint(parts[index], 0, 64)
				if err != nil {
					return byteJumpSpec{}, errors.Errorf("bitmask %q must be numeric", parts[index])
				}
				spec.hasBitmask = true
			default:
				return byteJumpSpec{}, errors.Errorf("unsupported option %q", option)
			}
		}
	}
	if spec.fromBeginning && spec.fromEnd {
		return byteJumpSpec{}, errors.New("from_beginning and from_end conflict")
	}
	return spec, nil
}

func parseByteExtract(raw string) (byteExtractSpec, error) {
	parts := strings.Split(raw, ",")
	if len(parts) < 3 {
		return byteExtractSpec{}, errors.New("requires bytes, offset and variable name")
	}
	for index := range parts {
		parts[index] = strings.TrimSpace(parts[index])
	}
	byteCount, err := strconv.Atoi(parts[0])
	if err != nil || byteCount < 0 || byteCount > 8 {
		return byteExtractSpec{}, errors.Errorf("byte count %q must be between 0 and 8", parts[0])
	}
	offset, err := strconv.Atoi(parts[1])
	if err != nil {
		return byteExtractSpec{}, errors.Errorf("offset %q must be numeric", parts[1])
	}
	if !isByteVariableName(parts[2]) {
		return byteExtractSpec{}, errors.Errorf("invalid variable name %q", parts[2])
	}
	spec := byteExtractSpec{bytes: byteCount, offset: offset, name: parts[2], multiplier: 1, base: 10}
	for index := 3; index < len(parts); index++ {
		option := parts[index]
		lower := strings.ToLower(option)
		switch lower {
		case "relative":
			spec.relative = true
		case "big":
			spec.little = false
		case "little":
			spec.little = true
		case "string":
			spec.stringMode = true
		case "dec":
			spec.base = 10
		case "hex":
			spec.base = 16
		case "oct":
			spec.base = 8
		case "align":
			spec.align = true
		case "dce":
			return byteExtractSpec{}, errors.New("dce byte order is not implemented")
		default:
			switch {
			case strings.HasPrefix(lower, "multiplier "):
				value := strings.TrimSpace(option[len("multiplier "):])
				spec.multiplier, err = strconv.Atoi(value)
				if err != nil || spec.multiplier < 0 {
					return byteExtractSpec{}, errors.Errorf("multiplier %q must be a non-negative integer", value)
				}
			case strings.HasPrefix(lower, "bitmask "):
				value := strings.TrimSpace(option[len("bitmask "):])
				spec.bitmask, err = strconv.ParseUint(value, 0, 64)
				if err != nil {
					return byteExtractSpec{}, errors.Errorf("bitmask %q must be numeric", value)
				}
				spec.hasBitmask = true
			default:
				return byteExtractSpec{}, errors.Errorf("unsupported option %q", option)
			}
		}
	}
	return spec, nil
}

func (s byteExtractSpec) extract(c *matchContext, mdf modifier.Modifier, buffer []byte) bool {
	position := s.offset
	if s.relative {
		prev, ok := c.GetPrevMatched(mdf)
		if !ok || len(prev) == 0 {
			return false
		}
		last := prev[len(prev)-1]
		position += last.Pos + last.Len
	}
	if position < 0 || position > len(buffer) {
		return false
	}
	var value uint64
	if s.bytes > 0 {
		if position+s.bytes > len(buffer) {
			return false
		}
		if s.stringMode {
			text := strings.TrimSpace(string(buffer[position : position+s.bytes]))
			text = strings.Trim(text, "\x00")
			text = strings.TrimPrefix(strings.TrimPrefix(text, "0x"), "0X")
			parsed, err := strconv.ParseUint(text, s.base, 64)
			if err != nil {
				return false
			}
			value = parsed
		} else {
			for index := 0; index < s.bytes; index++ {
				if s.little {
					value |= uint64(buffer[position+index]) << (8 * index)
				} else {
					value = value<<8 | uint64(buffer[position+index])
				}
			}
		}
	}
	if s.hasBitmask {
		value &= s.bitmask
	}
	value *= uint64(s.multiplier)
	if s.align && value%4 != 0 {
		value += 4 - value%4
	}
	c.variables[s.name] = int64(value)
	return true
}

func matchByteOps(c *matchContext, mdf modifier.Modifier, buffer []byte, specs []byteOpSpec) error {
	for _, spec := range specs {
		switch spec.kind {
		case "test":
			if !spec.test.matches(c, mdf, buffer) {
				c.Reject()
				return nil
			}
		case "extract":
			if !spec.extract.extract(c, mdf, buffer) {
				c.Reject()
				return nil
			}
		case "jump":
			target, ok := spec.jump.target(c, mdf, buffer)
			if !ok {
				c.Reject()
				return nil
			}
			if c.jumpCursor == nil {
				c.jumpCursor = make(map[modifier.Modifier]jumpCursorState)
			}
			c.jumpCursor[mdf] = jumpCursorState{position: target, active: true}
			c.SetPrevMatched(mdf, []data.Matched{{Pos: target, Len: 0}})
		}
	}
	return nil
}

func (s byteJumpSpec) target(c *matchContext, mdf modifier.Modifier, buffer []byte) (int, bool) {
	byteCount := s.bytes
	if s.bytesVar != "" {
		value, ok := c.variables[s.bytesVar]
		if !ok || value < 0 || value > 8 {
			return 0, false
		}
		byteCount = int(value)
	}
	readBase := 0
	if s.relative {
		prev, ok := c.GetPrevMatched(mdf)
		if !ok || len(prev) == 0 {
			return 0, false
		}
		last := prev[len(prev)-1]
		readBase = last.Pos + last.Len
	}
	readPosition := readBase + s.offset
	if readPosition < 0 || readPosition > len(buffer) {
		return 0, false
	}

	var extracted uint64
	if byteCount > 0 {
		if readPosition+byteCount > len(buffer) {
			return 0, false
		}
		if s.stringMode {
			text := strings.TrimSpace(string(buffer[readPosition : readPosition+byteCount]))
			text = strings.Trim(text, "\x00")
			text = strings.TrimPrefix(strings.TrimPrefix(text, "0x"), "0X")
			parsed, err := strconv.ParseUint(text, s.base, 64)
			if err != nil {
				return 0, false
			}
			extracted = parsed
		} else {
			for index := 0; index < byteCount; index++ {
				if s.little {
					extracted |= uint64(buffer[readPosition+index]) << (8 * index)
				} else {
					extracted = extracted<<8 | uint64(buffer[readPosition+index])
				}
			}
		}
	}
	if s.hasBitmask {
		extracted &= s.bitmask
	}
	extracted *= uint64(s.multiplier)
	if s.align && extracted%4 != 0 {
		extracted += 4 - extracted%4
	}

	landing := int64(extracted) + int64(s.postOffset)
	var target int
	switch {
	case s.fromBeginning:
		target = int(landing)
	case s.fromEnd:
		target = len(buffer) + int(landing)
	default:
		target = readPosition + int(landing) + byteCount
	}
	if target < 0 {
		target = 0
	}
	if target > len(buffer) {
		return 0, false
	}
	return target, true
}

func (s byteTestSpec) matches(c *matchContext, mdf modifier.Modifier, buffer []byte) bool {
	byteCount := s.bytes
	if s.bytesVar != "" {
		value, ok := c.variables[s.bytesVar]
		if !ok || value < 0 || value > 8 {
			return false
		}
		byteCount = int(value)
	}
	testValue := s.value
	if s.valueVar != "" {
		value, ok := c.variables[s.valueVar]
		if !ok || value < 0 {
			return false
		}
		testValue = uint64(value)
	}
	position := s.offset
	if s.relative {
		prev, ok := c.GetPrevMatched(mdf)
		if !ok || len(prev) == 0 {
			return false
		}
		last := prev[len(prev)-1]
		position += last.Pos + last.Len
	}
	if position < 0 {
		return false
	}

	var value uint64
	if s.stringMode {
		var data []byte
		if byteCount == 0 {
			if position > len(buffer) {
				return false
			}
			data = buffer[position:]
		} else {
			if position+byteCount > len(buffer) {
				return false
			}
			data = buffer[position : position+byteCount]
		}
		text := strings.TrimSpace(string(data))
		text = strings.Trim(text, "\x00")
		text = strings.TrimPrefix(strings.TrimPrefix(text, "0x"), "0X")
		if text == "" {
			return false
		}
		parsed, err := strconv.ParseUint(text, s.base, 64)
		if err != nil {
			return false
		}
		value = parsed
	} else {
		if byteCount <= 0 || position+byteCount > len(buffer) {
			return false
		}
		for index := 0; index < byteCount; index++ {
			if s.little {
				value |= uint64(buffer[position+index]) << (8 * index)
			} else {
				value = value<<8 | uint64(buffer[position+index])
			}
		}
	}
	if s.hasBitmask {
		value = (value & s.bitmask) >> s.shift
	}

	var matched bool
	switch s.operator {
	case "=", "!=":
		matched = value == testValue
	case "<":
		matched = value < testValue
	case ">":
		matched = value > testValue
	case "<=":
		matched = value <= testValue
	case ">=":
		matched = value >= testValue
	case "&":
		matched = value&testValue != 0
	case "^":
		matched = value^testValue != 0
	}
	if s.negate {
		matched = !matched
	}
	return matched
}

type base64DecodeSpec struct {
	bytes    int
	offset   int
	relative bool
}

func compileBase64Decode(raw string) (*base64DecodeSpec, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	spec := &base64DecodeSpec{bytes: 65535}
	parts := strings.Split(raw, ",")
	for index := 0; index < len(parts); index++ {
		part := strings.TrimSpace(parts[index])
		fields := strings.Fields(part)
		switch {
		case len(fields) == 1 && strings.EqualFold(fields[0], "relative"):
			spec.relative = true
		case len(fields) == 2 && strings.EqualFold(fields[0], "bytes"):
			value, err := strconv.Atoi(fields[1])
			if err != nil || value < 0 || value > 65535 {
				return nil, errors.Errorf("invalid base64_decode bytes %q", fields[1])
			}
			spec.bytes = value
		case len(fields) == 2 && strings.EqualFold(fields[0], "offset"):
			value, err := strconv.Atoi(fields[1])
			if err != nil || value < 0 {
				return nil, errors.Errorf("invalid base64_decode offset %q", fields[1])
			}
			spec.offset = value
		case len(fields) == 1 && strings.EqualFold(fields[0], "bytes") && index+1 < len(parts):
			index++
			value, err := strconv.Atoi(strings.TrimSpace(parts[index]))
			if err != nil || value < 0 || value > 65535 {
				return nil, errors.Errorf("invalid base64_decode bytes %q", parts[index])
			}
			spec.bytes = value
		case len(fields) == 1 && strings.EqualFold(fields[0], "offset") && index+1 < len(parts):
			index++
			value, err := strconv.Atoi(strings.TrimSpace(parts[index]))
			if err != nil || value < 0 {
				return nil, errors.Errorf("invalid base64_decode offset %q", parts[index])
			}
			spec.offset = value
		default:
			return nil, errors.Errorf("unsupported base64_decode option %q", part)
		}
	}
	return spec, nil
}

func (c *matchContext) applyBase64Decode(spec *base64DecodeSpec, mdf modifier.Modifier, buffer []byte) error {
	if spec == nil {
		return nil
	}
	position := spec.offset
	if spec.relative {
		prev, ok := c.GetPrevMatched(mdf)
		if !ok || len(prev) == 0 {
			c.SetBuffer(modifier.Base64Data, nil)
			return nil
		}
		last := prev[len(prev)-1]
		position += last.Pos + last.Len
	}
	if position < 0 || position >= len(buffer) {
		c.SetBuffer(modifier.Base64Data, nil)
		return nil
	}
	end := position + spec.bytes
	if end > len(buffer) {
		end = len(buffer)
	}
	c.SetBuffer(modifier.Base64Data, decodeBase64Prefix(buffer[position:end]))
	return nil
}

func decodeBase64Prefix(data []byte) []byte {
	end := 0
	for end < len(data) {
		ch := data[end]
		if ch >= 'A' && ch <= 'Z' || ch >= 'a' && ch <= 'z' ||
			ch >= '0' && ch <= '9' || ch == '+' || ch == '/' || ch == '=' {
			end++
			continue
		}
		break
	}
	if end == 0 {
		return nil
	}
	data = data[:end]
	if decoded, err := base64.StdEncoding.DecodeString(string(data)); err == nil {
		return decoded
	}
	if decoded, err := base64.RawStdEncoding.DecodeString(string(bytes.TrimRight(data, "="))); err == nil {
		return decoded
	}
	if decoded, _ := base64.StdEncoding.DecodeString(string(data)); len(decoded) > 0 {
		return decoded
	}
	return nil
}

func matchStandaloneIsDataAt(c *matchContext, buffer []byte, raw string) error {
	parts := strings.Split(raw, ",")
	if len(parts) != 1 {
		return errors.New("standalone isdataat does not support relative")
	}
	value := strings.TrimSpace(parts[0])
	negative := strings.HasPrefix(value, "!")
	value = strings.TrimPrefix(value, "!")
	position, err := strconv.Atoi(value)
	if err != nil || position < 0 {
		return errors.Wrap(err, "isdataat format error")
	}
	c.Must(negIf(negative, position < len(buffer)))
	return nil
}

// dotPrefixBuffer implements Suricata's dotprefix transform: the selected
// sticky buffer is prefixed with a literal dot so a content value such as
// ".example.com" can match the apex domain and its subdomains without also
// matching "notexample.com".
func dotPrefixBuffer(buffer []byte) []byte {
	prefixed := make([]byte, 1, len(buffer)+1)
	prefixed[0] = '.'
	return append(prefixed, buffer...)
}

func attachPayloadMatcher(c *matchContext) error {
	contentRules := c.Rule.ContentRuleConfig.ContentRules
	if len(contentRules) == 0 {
		if bsize, ok := c.Rule.SettingMap["bsize"]; ok {
			contentRules = append(contentRules, &rule.ContentRule{BSize: bsize})
		}
		if dsize, ok := c.Rule.SettingMap["dsize"]; ok {
			if len(contentRules) == 0 {
				contentRules = append(contentRules, &rule.ContentRule{DSize: dsize})
			} else {
				contentRules[0].DSize = dsize
			}
		}
	}
	for _, r := range contentRules {
		matcher, err := newPayloadMatcher(r, r.Modifier)
		if err != nil {
			return err
		}
		c.Attach(matcher)
	}
	if c.Rule.ContentRuleConfig.ASN1 != nil {
		c.Attach(asn1Matcher)
	}
	attachAppLayerMatchers(c)
	if c.Rule.ContentRuleConfig.FTPBounce {
		c.Attach(ftpBounceMatcher)
	}
	return nil
}

func attachFastPattern(c *matchContext) error {
	idx := slices.IndexFunc(c.Rule.ContentRuleConfig.ContentRules, func(rule *rule.ContentRule) bool {
		return rule.FastPattern && !rule.Negative
	})
	if idx == -1 {
		return nil
	}

	fastPatternRule := c.Rule.ContentRuleConfig.ContentRules[idx]
	fastPatternRule = fastPatternCopy(fastPatternRule)
	if fastPatternRule.Modifier == modifier.Base64Data {
		// The decoded buffer does not exist when prefilter handlers run; the
		// full payload workflow evaluates it after base64_decode.
		return nil
	}
	matcher, err := newPayloadMatcher(fastPatternRule, fastPatternRule.Modifier)
	if err != nil {
		return err
	}
	c.Attach(func(c *matchContext) error {
		prevMatched, prevModifier := c.prevMatched, c.prevModifier
		defer func() {
			c.prevMatched, c.prevModifier = prevMatched, prevModifier
		}()
		return matcher(c)
	})
	return nil
}
