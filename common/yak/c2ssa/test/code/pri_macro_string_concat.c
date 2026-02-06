#include <stdio.h>
#include <inttypes.h>

// Test PRI* macro string literal concatenation
// This is the most common error: strings like "%"PRId64 should be recognized
// as string concatenation, but the parser currently fails on this pattern.

int main() {
    int64_t value = 12345;
    uint32_t u32_value = 42;
    uint64_t u64_value = 67890;
    
    // Error case 1: Basic PRI* macro concatenation
    // This should parse as: "%" PRId64 -> "%" "ld" (after preprocessing)
    printf("Send frame %3"PRId64"\n", value);
    
    // Error case 2: Multiple PRI* in one format string
    printf("%"PRId64"(%"PRId64")\n", value, value);
    
    // Error case 3: PRIu32 concatenation
    printf("sample count mismatch: %"PRIu32" != %d\n", u32_value, 10);
    
    // Error case 4: PRIo64 concatenation
    char filemode[4];
    snprintf(filemode, 4, "%3"PRIo64, (uint64_t)value);
    
    // Error case 5: PRIx64 concatenation
    printf("0x%"PRIx64"\n", u64_value);
    
    // Error case 6: Complex format string with multiple PRI* macros
    printf("%-9s %12"PRId64" %30s %10s %s %16"PRId64" %16"PRId64" %16"PRId64"\n",
           "TYPE", value, "NAME", "UID(GID)", "UGO", value, value, value);
    
    // Error case 7: PRIu64 concatenation
    printf("version: %"PRIu64" and %"PRIu64"\n", u64_value, u64_value);
    
    return 0;
}

