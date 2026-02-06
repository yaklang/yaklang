// Test backslash line continuation in macros and strings
// Error case: token recognition error at: '\

#define MULTILINE_MACRO(a, b, c) \
    do { \
        int sum = (a) + (b) + (c); \
        printf("Sum: %d\n", sum); \
    } while(0)

#define ANOTHER_MACRO(x, y) \
    if ((x) > (y)) { \
        return (x); \
    } else { \
        return (y); \
    }

// String literal with line continuation
const char* long_string = "This is a very long string that \
spans multiple lines using backslash continuation";

// Function-like macro with line continuation
#define DEBUG_PRINT(fmt, ...) \
    do { \
        fprintf(stderr, "[DEBUG] " fmt "\n", ##__VA_ARGS__); \
    } while(0)

int main() {
    MULTILINE_MACRO(1, 2, 3);
    ANOTHER_MACRO(10, 20);
    DEBUG_PRINT("Test message: %s", "hello");
    return 0;
}

