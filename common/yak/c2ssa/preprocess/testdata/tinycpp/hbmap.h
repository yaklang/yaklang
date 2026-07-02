#ifndef HBMAP_H
#define HBMAP_H

/* this is a hashmap using a fixed number of buckets,
   which in turn are of type bmap. this combines the advantages of both
   approaches.
   limitations: max no of buckets and items per bucket is 2^32-1 each.
   speed is almost identical to khash with small number of items per
   bucket. with 100.000 items it's about 15% slower.

   unlike bmap, _find(), insert(), etc return an iterator instead of indices.
   the iterator needs to be used for e.g. _getkey(), etc.
*/

#include "bmap.h"
#include <stdint.h>
#include <stddef.h>
#include <stdlib.h>
#include <unistd.h> /* ssize_t */

#ifndef ARRAY_SIZE
#define ARRAY_SIZE(x) (sizeof(x) / sizeof((x)[0]))
#endif

typedef uint64_t hbmap_iter;

#define hbmap_impl(NAME, KEYTYPE, VALTYPE, NUMBUCKETS) \
struct NAME { \
	unsigned (*hash_func)(const KEYTYPE); \
	union { \
		hbmap_iter it; \
	} tmp; \
	bmap_impl(, KEYTYPE, VALTYPE) buckets[NUMBUCKETS]; \
}

#define hbmap(KEYTYPE, VALTYPE, NUMBUCKETS) \
	hbmap_impl(, KEYTYPE, VALTYPE, NUMBUCKETS)

#define hbmap_decl(ID, KEYTYPE, VALTYPE, NUMBUCKETS) \
	hbmap_impl(hbmap_ ## ID, KEYTYPE, VALTYPE, NUMBUCKETS)

#define hbmap_proto(NUMBUCKETS) \
	hbmap_impl(, void*, void*, NUMBUCKETS)

#define hbmap_getbucketcount(X) ARRAY_SIZE((X)->buckets)

#define hbmap_struct_size_impl(NUMBUCKETS) ( \
	offsetof(hbmap_proto(1), buckets) + \
	NUMBUCKETS * sizeof(bmap_proto) \
	)

#define hbmap_init_impl(X, COMPAREFUNC, HASHFUNC, NUMBUCKETS) do{\
	memset(X, 0, hbmap_struct_size_impl(NUMBUCKETS)); \
	((hbmap_proto(1)*)(void*)(X))->hash_func = (void*)HASHFUNC; \
	bmap_proto *p = (void*)(&((hbmap_proto(1)*)(void*)(X))->buckets[0]); \
	size_t i; for(i=0; i<NUMBUCKETS; ++i) \
		p[i].compare = COMPAREFUNC; \
	} while(0)

/* initialization */
/* bmap_compare_func is a typical compare function used for qsort, etc such as strcmp
 */

#define hbmap_init(X, COMPAREFUNC, HASHFUNC) \
	hbmap_init_impl(X, COMPAREFUNC, HASHFUNC, hbmap_getbucketcount(X))

static inline void* hbmap_new(bmap_compare_func fn, void* hash_func, size_t numbuckets) {
	void *nyu = malloc(hbmap_struct_size_impl(numbuckets));
	if(nyu) hbmap_init_impl(nyu, fn, hash_func, numbuckets);
	return nyu;
}

/* destruction */
/* freeflags:
  0: free only internal mem
  1: 0+free all keys,
  2: 0+free all values,
  3: 0+free both
*/
#define hbmap_fini(X, FREEFLAGS) do { \
	size_t i; for(i=0; i < hbmap_getbucketcount(X); ++i) \
		{ bmap_fini(&(X)->buckets[i], FREEFLAGS); } \
} while(0)

/* internal stuff needed for iterator impl */

#define hbmap_iter_bucket(I) ( (I) >> 32)
#define hbmap_iter_index(I)  ( (I) & 0xffffffff )
#define hbmap_iter_makebucket(I) ( (I) << 32)

#define hbmap_iter_bucket_valid(X, ITER, NUMBUCKETS) ( \
	hbmap_iter_bucket(ITER) < NUMBUCKETS )
#define hbmap_iter_index_valid(X, ITER) ( \
	hbmap_iter_index(ITER) < bmap_getsize(& \
	(((bmap_proto *)( \
	(void*)(&((hbmap_proto(1)*)(void*)(X))->buckets[0]) \
	))[hbmap_iter_bucket(ITER)]) \
	))

#define hbmap_iter_valid(X, ITER) (\
	hbmap_iter_bucket_valid(X, ITER, hbmap_getbucketcount(X)) && \
	hbmap_iter_index_valid(X, ITER))

#define hbmap_next_step(X, ITER) ( \
	hbmap_iter_index_valid(X, (ITER)+1) ? (ITER)+1 : \
	hbmap_iter_makebucket(hbmap_iter_bucket(ITER)+1) \
	)

static hbmap_iter hbmap_next_valid_impl(void *h, hbmap_iter iter, size_t nbucks) {
	do iter = hbmap_next_step(h, iter);
	while(hbmap_iter_bucket_valid(h, iter, nbucks) && !hbmap_iter_index_valid(h, iter));
	return iter;
}

/* public API continues */

/* note that if you use foreach to delete items, the iterator isn't aware of that
   and will skip over the next item. you need to use something like:
   hbmap_foreach(map, i) { while(hbmap_iter_index_valid(map, i)) hbmap_delete(map, i); }
*/
#define hbmap_foreach(X, ITER_VAR) \
	for(ITER_VAR = hbmap_iter_valid(X, (hbmap_iter)0) ? 0 \
	: hbmap_next_valid_impl(X, 0, hbmap_getbucketcount(X)); \
		hbmap_iter_valid(X, ITER_VAR); \
		ITER_VAR = hbmap_next_valid_impl(X, ITER_VAR, hbmap_getbucketcount(X)))

#define hbmap_getkey(X, ITER) \
	bmap_getkey(&(X)->buckets[hbmap_iter_bucket(ITER)], hbmap_iter_index(ITER))

#define hbmap_getval(X, ITER) \
	bmap_getval(&(X)->buckets[hbmap_iter_bucket(ITER)], hbmap_iter_index(ITER))

#define hbmap_setvalue(X, VAL, ITER) \
	bmap_setvalue(&(X)->buckets[hbmap_iter_bucket(ITER)], VAL, hbmap_iter_index(ITER))

#define hbmap_getkeysize(X) (bmap_getkeysize(&(X)->buckets[0]))
#define hbmap_getvalsize(X) (bmap_getvalsize(&(X)->buckets[0]))

#define hbmap_buckindex_impl(X, KEY) \
	( (hbmap_iter) (X)->hash_func(KEY) % hbmap_getbucketcount(X) )

#define hbmap_find(X, KEY) ( \
	( (X)->tmp.it = hbmap_iter_makebucket(hbmap_buckindex_impl(X, KEY) ) ), \
	((X)->tmp.it |= (int64_t) bmap_find(&(X)->buckets[ hbmap_iter_bucket((X)->tmp.it) ], KEY)), \
	(X)->tmp.it)

#define hbmap_contains(X, KEY) (hbmap_find(X, KEY) != (hbmap_iter)-1)

/* unlike hbmap_getkey/val with index, this returns a pointer-to-item, or NULL */
#define hbmap_get(X, KEY) ( \
	( hbmap_find(X, KEY) == (hbmap_iter) -1 ) ? 0 : &hbmap_getval(X, (X)->tmp.it) \
	)

/* same as hbmap_insert, but inserts blindly without checking for existing items.
   this is faster and can be used when it's impossible that duplicate
   items are added */
#define hbmap_insert_nocheck(X, KEY, VAL) ( \
	( (X)->tmp.it = hbmap_iter_makebucket(hbmap_buckindex_impl(X, KEY) ) ), \
	((X)->tmp.it |= (int64_t) bmap_insert_nocheck(&(X)->buckets[hbmap_iter_bucket((X)->tmp.it)], KEY, VAL)), \
	(X)->tmp.it)

/* insert item into mapping, overwriting existing items with the same key */
/* return index of new item, or -1. overwrites existing items. */
#define hbmap_insert(X, KEY, VAL) ( \
		( hbmap_find(X, KEY) == (hbmap_iter) -1 ) ? hbmap_insert_nocheck(X, KEY, VAL) : \
		( hbmap_setvalue(X, VAL, (X)->tmp.it), (X)->tmp.it ) \
	)

#define hbmap_delete(X, ITER) ( \
	bmap_delete(&(X)->buckets[hbmap_iter_bucket(ITER)], hbmap_iter_index(ITER)), 1)

#endif
