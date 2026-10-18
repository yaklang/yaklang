package minirehs

import (
	"bytes"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"hash/fnv"
	"regexp/syntax"
)

var (
	ErrUnsupportedImage  = errors.New("minirehs: images require the MVS backend")
	ErrIncompatibleImage = errors.New("minirehs: incompatible compilation image")
	ErrCorruptImage      = errors.New("minirehs: corrupt compilation image")
)

const imageVersion uint32 = 1
const imageHeaderSize = 32

const (
	imageReportLocation uint32 = 1 << iota
	imageBiAnchor
	imageAnchorMerged
	imageAssertMerged
)

const (
	imageHasLiterals uint32 = 1 << iota
	imageWindowable
	imageBatchable
	imageAnchorable
	imageBiAnchorable
	imageGate
	imageRE2Loc
)

// Hooks are nil outside tests. All frontend syntax parsing passes through the
// wrapper, so the load-without-compilation contract is observable in tests.
var imageCompileHook, imageNFAHook, imageParseHook func()

func parseRegexSyntax(expr string, flags syntax.Flags) (*syntax.Regexp, error) {
	if imageParseHook != nil {
		imageParseHook()
	}
	return syntax.Parse(expr, flags)
}

func compileImageFlags(cfg *config) uint32 {
	var f uint32
	if cfg.reportLocation {
		f |= imageReportLocation
	}
	if biAnchorEnabled {
		f |= imageBiAnchor
	}
	if anchorMergedEnabled {
		f |= imageAnchorMerged
	}
	if assertMergedEnabled {
		f |= imageAssertMerged
	}
	return f
}

func imageIdentity(patterns []Pattern, cfg *config, flags uint32) uint64 {
	h := fnv.New64()
	var buf [12]byte
	for _, p := range patterns {
		binary.LittleEndian.PutUint32(buf[0:4], uint32(p.ID))
		binary.LittleEndian.PutUint32(buf[4:8], uint32(p.Flags))
		binary.LittleEndian.PutUint32(buf[8:12], uint32(len(p.Expr)))
		_, _ = h.Write(buf[:])
		_, _ = h.Write([]byte(p.Expr))
	}
	for _, v := range []uint32{uint32(cfg.minLiteralLen), flags, imageVersion, mvsBlobVersion} {
		binary.LittleEndian.PutUint32(buf[:4], v)
		_, _ = h.Write(buf[:4])
	}
	return h.Sum64()
}

func (d *database) imageMVS() *mvsDB {
	if d == nil {
		return nil
	}
	if m, ok := d.primary.(*mvsDB); ok {
		return m
	}
	if c, ok := d.primary.(*compositeDB); ok && c.fallback == nil {
		m, _ := c.primary.(*mvsDB)
		return m
	}
	return nil
}

// MarshalImage exports a deterministic, little-endian compilation image.
// The returned bytes own their storage. No regex is parsed or compiled here.
func (d *database) MarshalImage() ([]byte, error) {
	m := d.imageMVS()
	if m == nil {
		return nil, ErrUnsupportedImage
	}
	if m.minLiteralLen < 1 || m.minLiteralLen > 0x7fffffff {
		return nil, ErrUnsupportedImage
	}
	if m.imageFlags & ^uint32(imageReportLocation|imageBiAnchor) != 0 {
		return nil, ErrIncompatibleImage
	}
	reverse := make([][]byte, m.n)
	nrev := 0
	for i, n := range m.revNFAs {
		if n != nil {
			reverse[i] = encodeUnit(unitFromNFA(n, i, true))
			nrev++
		}
	}
	preconditions := encodeImagePreconditions(m)
	size := imageEncodedSize(m, reverse) + 4 + uint64(len(preconditions))
	if size > 0x7fffffff {
		return nil, ErrUnsupportedImage
	}
	b := make([]byte, 0, int(size))
	b = append(b, 'R', 'E', 'H', 'S')
	for _, v := range []uint32{imageVersion, mvsBlobVersion, m.imageFlags, uint32(m.minLiteralLen)} {
		b = putU32(b, v)
	}
	b = putU64(b, m.patternHash)
	b = putU32(b, uint32(m.n))
	kinds := make([]mvsSpecializedKind, m.n)
	for _, p := range m.specializedAlwaysOn {
		kinds[p.idx] = p.kind
	}
	for i, cp := range m.all {
		b = putU32(b, uint32(cp.id))
		b = putU32(b, uint32(cp.flags))
		if cp.v.exact() {
			b = append(b, 0)
		} else {
			b = append(b, 1)
		}
		var route uint32
		for j, on := range []bool{m.hasLiterals[i], m.windowable[i], m.batchable[i], m.anchorable[i], m.biAnchorable[i], m.gate[i], m.re2Loc[i]} {
			if on {
				route |= 1 << uint(j)
			}
		}
		b = putU32(b, route)
		b = append(b, byte(kinds[i]))
		for _, v := range []int32{int32(cp.winW), m.gateHead[i], m.win[i].head, m.win[i].tail} {
			b = putI32(b, v)
		}
		b = imagePutString(b, cp.expr)
		b = imagePutStrings(b, cp.literals)
	}
	b = imagePutStrings(b, m.literals)
	for i, pats := range m.litToPat {
		for _, vs := range [][]int32{pats, m.litHead[i], m.litBiHead[i], m.litBiTail[i]} {
			b = imagePutInts(b, vs)
		}
	}
	b = putU32(b, uint32(m.mergedCount))
	b = putU32(b, uint32(m.maxAnchorNword))
	b = imagePutIndexes(b, m.assertAlwaysOn)
	b = imagePutInts(b, m.assertAlwaysOnCIdxs)
	b = imagePutIndexes(b, m.assertAlwaysOnGoIdxs)
	b = imagePutIndexes(b, m.otherAlwaysOn)
	b = putU32(b, uint32(len(m.specializedAlwaysOn)))
	for _, p := range m.specializedAlwaysOn {
		b = putI32(b, int32(p.idx))
		b = append(b, byte(p.kind))
	}
	b = putU32(b, uint32(nrev))
	for i, raw := range reverse {
		if raw != nil {
			b = putI32(b, int32(i))
			b = imagePutBytes(b, raw)
		}
	}
	b = imagePutBytes(b, m.blob)
	// v1 leaves regexp2 verifiers lazy. Its implementation-dependent bytecode is
	// deliberately absent from this portable format.
	for i, cp := range m.all {
		if !cp.v.exact() {
			b = putI32(b, int32(i))
			b = putU32(b, 0)
		}
	}
	b = imagePutBytes(b, preconditions)
	if uint64(len(b)) > 0x7ffffffb {
		return nil, ErrUnsupportedImage
	}
	return putU32(b, crc32.ChecksumIEEE(b)), nil
}

// Reserve the whole image, including routing metadata, before appending the
// large native blob. Growing a blob-sized slice here would copy tens of MiB.
func imageEncodedSize(m *mvsDB, reverse [][]byte) uint64 {
	n := uint64(len(m.blob)) + 76
	for _, cp := range m.all {
		n += 38 + uint64(len(cp.expr))
		for _, lit := range cp.literals {
			n += 4 + uint64(len(lit))
		}
		if !cp.v.exact() {
			n += 8
		}
	}
	for i, lit := range m.literals {
		n += 20 + uint64(len(lit)) + 16*uint64(len(m.litToPat[i]))
	}
	n += 4 * uint64(len(m.assertAlwaysOn)+len(m.assertAlwaysOnCIdxs)+len(m.assertAlwaysOnGoIdxs)+len(m.otherAlwaysOn))
	n += 5 * uint64(len(m.specializedAlwaysOn))
	for _, raw := range reverse {
		if raw != nil {
			n += 8 + uint64(len(raw))
		}
	}
	return n
}

type compilationImageHeader struct {
	flags         uint32
	minLiteralLen int
	hash          uint64
	n             int
}

func readImageHeader(data []byte) (compilationImageHeader, error) {
	var h compilationImageHeader
	if len(data) < imageHeaderSize+4 || !bytes.Equal(data[:4], []byte("REHS")) {
		return h, ErrCorruptImage
	}
	if crc32.ChecksumIEEE(data[:len(data)-4]) != binary.LittleEndian.Uint32(data[len(data)-4:]) {
		return h, ErrCorruptImage
	}
	if binary.LittleEndian.Uint32(data[4:]) != imageVersion || binary.LittleEndian.Uint32(data[8:]) != mvsBlobVersion {
		return h, ErrIncompatibleImage
	}
	h.flags = binary.LittleEndian.Uint32(data[12:])
	if h.flags & ^uint32(imageReportLocation|imageBiAnchor) != 0 {
		return h, ErrIncompatibleImage
	}
	h.minLiteralLen = int(binary.LittleEndian.Uint32(data[16:]))
	h.hash = binary.LittleEndian.Uint64(data[20:])
	h.n = int(binary.LittleEndian.Uint32(data[28:]))
	if h.minLiteralLen < 1 || h.minLiteralLen > 0x7fffffff || h.n < 1 || h.n > (len(data)-imageHeaderSize)/38 {
		return h, ErrCorruptImage
	}
	return h, nil
}

// LoadImage opens an MVS compilation image without compiling regexes. The input
// can be reused or overwritten after return. Verification-only expressions are
// compiled with sync.Once when a scan first needs them. CRC32 detects accidental
// corruption, not authenticity; distribute embedded images with the binary.
func LoadImage(data []byte) (Database, error) {
	h, err := readImageHeader(data)
	if err != nil {
		return nil, err
	}
	return loadCompilationImage(data, h)
}

func loadCompilationImage(data []byte, h compilationImageHeader) (Database, error) {
	var err error
	r := &imageReader{b: data[:len(data)-4], off: imageHeaderSize}
	d := &mvsDB{n: h.n, imageFlags: h.flags, minLiteralLen: h.minLiteralLen, patternHash: h.hash, reportLoc: h.flags&imageReportLocation != 0,
		all: make([]*compiledPattern, h.n), nfas: make([]*mvsNFA, h.n), revNFAs: make([]*mvsNFA, h.n),
		gate: make([]bool, h.n), re2Loc: make([]bool, h.n), windowable: make([]bool, h.n), batchable: make([]bool, h.n),
		anchorable: make([]bool, h.n), biAnchorable: make([]bool, h.n), hasLiterals: make([]bool, h.n),
		win: make([]litWindow, h.n), gateHead: make([]int32, h.n), assertNecFactor: make([]necFactor, h.n)}
	kinds := make([]mvsSpecializedKind, h.n)
	reports := make([]PatternReport, h.n)
	for i := range d.all {
		cp := &compiledPattern{idx: i, id: PatternID(r.u32()), flags: Flag(r.u32())}
		vk := r.u8()
		route := r.u32()
		kinds[i] = mvsSpecializedKind(r.u8())
		cp.winW = int(r.i32())
		d.gateHead[i], d.win[i].head, d.win[i].tail = r.i32(), r.i32(), r.i32()
		cp.expr, cp.literals = r.str(), r.strings()
		if r.err != nil || cp.expr == "" || vk > 1 || route & ^uint32(127) != 0 || kinds[i] > mvsSpecializedWindowsPath ||
			cp.winW < 0 || d.gateHead[i] < -1 || d.win[i].head < -1 || d.win[i].tail < -1 {
			return nil, ErrCorruptImage
		}
		if vk == 0 {
			cp.v = &re2Verifier{expr: cp.expr, longest: d.reportLoc}
		} else {
			cp.v = &regexp2Verifier{expr: cp.expr}
		}
		for j, dst := range [][]bool{d.hasLiterals, d.windowable, d.batchable, d.anchorable, d.biAnchorable, d.gate, d.re2Loc} {
			dst[i] = route&(1<<uint(j)) != 0
		}
		if d.hasLiterals[i] != (len(cp.literals) > 0) {
			return nil, ErrCorruptImage
		}
		d.all[i] = cp
		reports[i] = PatternReport{ID: cp.id, Disposition: dispositionOf(cp), HasLiteral: d.hasLiterals[i]}
	}
	d.literals = r.strings()
	nl := len(d.literals)
	d.litToPat, d.litHead, d.litBiHead, d.litBiTail = make([][]int32, nl), make([][]int32, nl), make([][]int32, nl), make([][]int32, nl)
	for i := range d.literals {
		d.litToPat[i], d.litHead[i], d.litBiHead[i], d.litBiTail[i] = r.intList(), r.intList(), r.intList(), r.intList()
	}
	d.mergedCount, d.maxAnchorNword = int(r.u32()), int(r.u32())
	d.assertAlwaysOn = imageIndexes(r.intList())
	d.assertAlwaysOnCIdxs = r.intList()
	d.assertAlwaysOnGoIdxs = imageIndexes(r.intList())
	d.otherAlwaysOn = imageIndexes(r.intList())
	ns := r.count(5)
	for i := 0; i < ns; i++ {
		d.specializedAlwaysOn = append(d.specializedAlwaysOn, mvsSpecializedAlwaysOn{idx: int(r.i32()), kind: mvsSpecializedKind(r.u8())})
	}
	nr := r.count(8)
	for i := 0; i < nr; i++ {
		idx := int(r.i32())
		raw := r.bytes()
		if idx < 0 || idx >= d.n || d.revNFAs[idx] != nil {
			return nil, ErrCorruptImage
		}
		u, e := decodeImageUnit(raw, d.n)
		if e != nil || u.hasAssert || u.hasDFA || u.hasLimEx || u.hasAnchored || !bsIsZero(u.lastEnd) {
			return nil, ErrCorruptImage
		}
		d.revNFAs[idx] = nfaFromImageUnit(u)
		d.hasBiAnchor = true
	}
	d.blob = bytes.Clone(r.bytes())
	for i, cp := range d.all {
		if !cp.v.exact() {
			if r.i32() != int32(i) {
				return nil, ErrCorruptImage
			}
			code := r.bytes()
			if r.err != nil {
				return nil, r.err
			}
			if len(code) != 0 {
				return nil, ErrIncompatibleImage
			}
		}
	}
	preconditions := r.bytes()
	if !r.done() {
		return nil, ErrCorruptImage
	}
	if err = decodeImageBlob(d.blob, d); err != nil {
		return nil, err
	}
	if err = validateImageRoutes(d, kinds); err != nil {
		return nil, err
	}
	if err = decodeImagePreconditions(preconditions, d); err != nil {
		return nil, err
	}
	if mvsKernelAvailable() {
		d.kernel = openMVSKernel(d.blob, d.n)
		if d.kernel == nil {
			return nil, ErrCorruptImage
		}
	}
	if nl > 0 {
		d.pf = newPrefilter(&literalIndex{literals: d.literals, litToPat: d.litToPat})
	}
	info := DatabaseInfo{Backend: BackendMVS, Tier: (&mvsBackend{}).tier(), NumPatterns: d.n, NumAlwaysOn: d.numAlwaysOn(), Reports: reports}
	return &database{primary: newCompositeDB(d, nil), info: info}, nil
}
