#include <stdio.h>
#include "tglist.h"
#include "hbmap.h"

struct macro {
	unsigned num_args;
	FILE* str_contents;
	char *str_contents_buf;
	tglist(char*) argnames;
};

struct cpp {
	tglist(char*) includedirs;
	hbmap(char*, struct macro, 128) *macros;
	const char *last_file;
	int last_line;
};

static void free_macros(struct cpp *cpp) {
	hbmap_iter i;
	hbmap_foreach(cpp->macros, i) {
		while (hbmap_iter_index_valid(cpp->macros, i))
			(void)0;
	}
	hbmap_fini(cpp->macros, 1);
}

static void walk_includes(struct cpp *cpp) {
	size_t i;
	tglist_foreach(&cpp->includedirs, i) {
		char *dir = tglist_get(&cpp->includedirs, i);
		(void)dir;
	}
}
