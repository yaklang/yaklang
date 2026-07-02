#ifndef BMAP_H
#define BMAP_H

/* this is a map behaving like a hashmap, but it uses binary search
   on a sorted list behind the curtains. this allows to find the
   required entry very quickly, while avoiding a lot of the complexity
   of a hashmap.

   for e.g. 10000 array elements, a binary search will only require 12
   comparisons, whereas a hashmap needs to compute a hash, then find
   a corresponding bucket and iterate over the bucket items, of which
   there could be multiple as well. so it shouldn't be much faster,
   while having a lot more overhead in code and RAM.

   since we use our per-value tglist behind the scenes, which looks
   basically like a flat array of the stored items, there's almost
   zero memory overhead with our method here.

   the slowest part is insertion of the item into the list, which uses
   memmove on a single block of memory. this is no problem when the
   total number of entries is relatively small.

   when comparing to khash, which is known as one of the fastest hashmap
   implementations due to usage of macros to generate inlined code,
   we reach about 80-75% of its speed with around 1000 items, but only
   40% with 3000, 33% with 10000 etc. the more items, the slower it
   becomes in comparison.

   so for the vast majority of tasks, this implementation provides
   speed comparable to the fastest hashmap implementation, while adding
   only a few hundred byte to binary size. a size-optimized benchmark
   program with bmap is 5.5KB, an equivalent one with kash 9.6KB.

   an additional advantage is that the map can be iterated like a
   regular array, which is already sorted by key.

*/

#include "tglist.h"
#include <stdint.h>
#include <stddef.h>
#include <stdlib.h>
#include <unistd.h> /* ssize_t */

#define bmap_cat(a, b) bmap_cat_impl(a, b)
#define bmap_cat_impl(a, b) a ## b

#ifdef BMAP_USE_TGILIST
#include "tgilist.h"
#define VAL_LIST_TYPE tgilist
#define VAL_LIST_ARGCALL(FN, A, B, C) FN(A, B, C)
#else
#define VAL_LIST_TYPE tglist
#define VAL_LIST_ARGCALL(FN, A, B, C) FN(A, B)
#endif


typedef int (*bmap_compare_func)(const void *, const void *);

#define bmap_impl(NAME, KEYTYPE, VALTYPE) \
struct NAME { \
	tglist_impl(, KEYTYPE) keys; \
	VAL_LIST_ARGCALL(bmap_cat(VAL_LIST_TYPE, _impl), ,VALTYPE, unsigned) values; \
	bmap_compare_func compare; \
	union { \
		KEYTYPE* kt; \
		VALTYPE* vt; \
		ssize_t ss; \
	} tmp; \
}

#define bmap(KEYTYPE, VALTYPE) bmap_impl(, KEYTYPE, VALTYPE)
#define bmap_decl(ID, KEYTYPE, VALTYPE) bmap_impl(bmap_ ## ID, KEYTYPE, VALTYPE)
#define bmap_proto bmap_impl(, void*, void*)

/* initialization */
/* bmap_compare_func is a typical compare function used for qsort, etc such as strcmp
 */
#define bmap_init(X, COMPAREFUNC) do{\
	memset(X, 0, sizeof(*(X))); \
	(X)->compare = COMPAREFUNC; } while(0)

static inline void* bmap_new(bmap_compare_func fn) {
	bmap_proto *nyu = malloc(sizeof(bmap_proto));
	if(nyu) bmap_init(nyu, fn);
	return nyu;
}

/* destruction */
/* freeflags:
  0: free only internal mem
  1: 0+free all keys,
  2: 0+free all values,
  3: 0+free both
*/
#define bmap_fini(X, FREEFLAGS) do { \
	if(FREEFLAGS & 1) {tglist_free_values(&(X)->keys);} \
	if(FREEFLAGS & 2) {bmap_cat(VAL_LIST_TYPE, _free_values)(&(X)->values);} \
	tglist_free_items(&(X)->keys); \
	bmap_cat(VAL_LIST_TYPE, _free_items)(&(X)->values); \
} while(0)

/* set value when key index is known. returns int 0 on failure, 1 on succ.*/
#define bmap_setvalue(B, VAL, POS) bmap_cat(VAL_LIST_TYPE, _set)(&(B)->values, VAL, POS)

#define bmap_getsize(B) tglist_getsize(&(B)->keys)
#define bmap_getkey(B, X) tglist_get(&(B)->keys, X)
#define bmap_getval(B, X) bmap_cat(VAL_LIST_TYPE, _get)(&(B)->values, X)
#define bmap_getkeysize(B) (tglist_itemsize(&(B)->keys))
#define bmap_getvalsize(B) (bmap_cat(VAL_LIST_TYPE, _itemsize)(&(B)->values))

#define bmap_find(X, KEY) \
	( (X)->tmp.kt = (void*)&(KEY), bmap_find_impl(X, (X)->tmp.kt, bmap_getkeysize(X)) )

#define bmap_contains(B, KEY) (bmap_find(B, KEY) != (ssize_t)-1)

/* unlike bmap_getkey/val with index, this returns a pointer-to-item, or NULL */
#define bmap_get(X, KEY) \
	( (((X)->tmp.kt = (void*)&(KEY)), 1) &&  \
	( (X)->tmp.ss = bmap_find_impl(X, (X)->tmp.kt, bmap_getkeysize(X)) ) == (ssize_t) -1 ? \
		0 : &bmap_getval(X, (X)->tmp.ss) )

/* same as bmap_insert, but inserts blindly without checking for existing items.
   this is faster and can be used when it's impossible that duplicate
   items are added */
#define bmap_insert_nocheck(X, KEY, VAL) ( \
	(  \
	(  (X)->tmp.ss = tglist_insert_sorted(&(X)->keys, KEY, (X)->compare) ) \
		== (ssize_t) -1) ? (ssize_t) -1 : ( \
			bmap_cat(VAL_LIST_TYPE, _insert)(&(X)->values, VAL, (X)->tmp.ss) ? (X)->tmp.ss : \
			(  tglist_delete(&(X)->keys, (X)->tmp.ss), (ssize_t) -1  ) \
		) \
	)
/* insert item into mapping, overwriting existing items with the same key */
/* return index of new item, or -1. overwrites existing items. */
// FIXME evaluates KEY twice
#define bmap_insert(X, KEY, VAL) ( \
		( (X)->tmp.ss = bmap_find(X, KEY) ) \
		== (ssize_t) -1 ? bmap_insert_nocheck(X, KEY, VAL) : \
		bmap_cat(VAL_LIST_TYPE, _set)(&(X)->values, VAL, (X)->tmp.ss), (X)->tmp.ss \
	)

#define bmap_delete(X, POS) ( \
	(X)->tmp.ss = POS, \
	tglist_delete(&(X)->keys, POS), \
	bmap_cat(VAL_LIST_TYPE, _delete)(&(X)->values, POS) \
	)

static ssize_t bmap_find_impl(void* bm, const void* key, size_t keysize) {
	bmap_proto *b = bm;
	void *r = bsearch(key, b->keys.items, bmap_getsize(b), keysize, b->compare);
	if(!r) return -1;
	return ((uintptr_t) r - (uintptr_t) b->keys.items)/keysize;
}

#endif
