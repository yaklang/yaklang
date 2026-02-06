#include <stdio.h>
#include <inttypes.h>

// Test string literal concatenation
// Note: In real code, PRId64, PRIu64, etc. are macros that expand to string literals
// during preprocessing. After preprocessing, they become adjacent string literals
// which are automatically concatenated by the C compiler.
// This test shows the preprocessed form (what the parser actually sees).

int main() {
    int64_t value = 12345;
    
    // String literal concatenation (preprocessed form)
    // After preprocessing, PRId64 expands to "ld", PRIu64 to "lu", etc.
    // So "%" PRId64 becomes "%" "ld" which concatenates to "%ld"
    printf("%" "ld" "\n", value);
    printf("%" "lu" "\n", (uint64_t)value);
    printf("%" "lx" "\n", (uint64_t)value);
    printf("%" "lo" "\n", (uint64_t)value);
    
    // Multiple concatenations
    printf("%3" "lo" "\n", (uint64_t)value);
    printf("%12" "ld" "\n", value);
    
    // In function calls
    char buffer[100];
    snprintf(buffer, sizeof(buffer), "%" "ld", value);
    
    // Complex format string with multiple concatenations
    printf("%-9s %12" "ld" " %30s\n", "TYPE", value, "NAME");
    
    return 0;
}

