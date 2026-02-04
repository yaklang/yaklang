#include <stdio.h>

// Test complex declarations that might cause parsing issues

// Function pointer with const
typedef int (*func_ptr)(const char *);

// Const pointer to function
int (* const const_func_ptr)(void) = NULL;

// Pointer to const function pointer
int (*(*ptr_to_func_ptr))(void);

// Array of function pointers
int (*func_array[10])(int, int);

// Function returning pointer to array
int (*return_array_ptr(void))[10];

// Function returning pointer to function
int (*(*return_func_ptr(void))(int))(int);

// Complex const combinations
const int * const * const complex_ptr = NULL;
int const * const * const complex_ptr2 = NULL;

// Nested pointer types
int ***triple_ptr;
const int ***const_triple_ptr;

int main() {
    // Variable declarations with complex types
    struct AVCodecDescriptor const *desc = NULL;
    AVCodec const *codec = NULL;
    
    return 0;
}

