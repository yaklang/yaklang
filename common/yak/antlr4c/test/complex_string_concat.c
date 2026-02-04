#include <stdio.h>
#include <inttypes.h>

// Test complex string literal concatenation scenarios
// Multiple adjacent strings and identifiers that should concatenate

int main() {
    int64_t value = 12345;
    uint32_t u32 = 42;
    
    // Error case 1: String + identifier + string pattern
    printf("%"PRId64"(%"PRId64")\n", value, value);
    
    // Error case 2: Multiple identifiers in format string
    printf("%-9s %12"PRId64" %30s %10s %s %16"PRId64" %16"PRId64" %16"PRId64"\n",
           "TYPE", value, "SIZE", "NAME", "UID(GID)", value, value, value);
    
    // Error case 3: In snprintf calls
    char buffer[256];
    snprintf(buffer, sizeof(buffer), "uid_and_gid: %"PRId64"(%"PRId64")", value, value);
    
    // Error case 4: Mixed with regular strings
    printf("File: %s, Size: %"PRId64", Mode: %"PRIo64"\n", "test.txt", value, (uint64_t)value);
    
    // Error case 5: Complex nested format
    printf("time_base=%d/%d:sample_rate=%d:sample_fmt=%s:channel_layout=0x%"PRIx64"\n",
           1, 1, 44100, "s16", (uint64_t)value);
    
    return 0;
}

