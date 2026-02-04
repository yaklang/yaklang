#include <stdio.h>
#include <stddef.h>
#include <inttypes.h>

// Test PTRDIFF_SPECIFIER macro (similar to PRI* macros)
// This is used for ptrdiff_t printf formatting

int main() {
    int arr[10] = {0};
    int* p1 = &arr[0];
    int* p2 = &arr[5];
    ptrdiff_t diff = p2 - p1;
    
    // Error case: PTRDIFF_SPECIFIER string concatenation
    // After preprocessing, this becomes something like "%" "td"
    printf("Pointer difference: %"PRIdPTR"\n", diff);
    
    // Alternative form (if PRIdPTR is not available)
    printf("begin %"PRIdPTR" smaller than time diff index %d.\n", diff, 5);
    printf("end %"PRIdPTR" smaller than time diff index %d.\n", diff, 3);
    
    return 0;
}

