#ifndef PREPROC_H
#define PREPROC_H

#include <stdio.h>

struct cpp;

struct cpp *cpp_new(void);
void cpp_free(struct cpp*);
void cpp_add_includedir(struct cpp *cpp, const char* includedir);
int cpp_add_define(struct cpp *cpp, const char *mdecl);
int cpp_run(struct cpp *cpp, FILE* in, FILE* out, const char* inname);

#ifdef __GNUC__
#pragma GCC diagnostic ignored "-Wunknown-pragmas"
#endif
#pragma RcB2 DEP "preproc.c"

#endif

