package yakgit

import "github.com/go-git/go-git/v5/plumbing"

type observerObject struct {
	hash   string
	otype  plumbing.ObjectType
	size   int64
	offset int64
	crc    uint32
}

type packObserver struct {
	count    uint32
	checksum string
	objects  []observerObject
	pos      map[int64]int
}

func (t *packObserver) OnHeader(count uint32) error {
	t.count = count
	t.pos = make(map[int64]int, count)
	return nil
}

func (t *packObserver) OnInflatedObjectHeader(otype plumbing.ObjectType, objSize int64, pos int64) error {
	o := t.get(pos)
	o.otype = otype
	o.size = objSize
	o.offset = pos

	t.put(pos, o)

	return nil
}

func (t *packObserver) OnInflatedObjectContent(h plumbing.Hash, pos int64, crc uint32, _ []byte) error {
	o := t.get(pos)
	o.hash = h.String()
	o.crc = crc

	t.put(pos, o)

	return nil
}

func (t *packObserver) OnFooter(h plumbing.Hash) error {
	t.checksum = h.String()
	return nil
}

func (t *packObserver) get(pos int64) observerObject {
	i, ok := t.pos[pos]
	if ok {
		return t.objects[i]
	}

	return observerObject{}
}

func (t *packObserver) put(pos int64, o observerObject) {
	i, ok := t.pos[pos]
	if ok {
		t.objects[i] = o
		return
	}

	t.pos[pos] = len(t.objects)
	t.objects = append(t.objects, o)
}
