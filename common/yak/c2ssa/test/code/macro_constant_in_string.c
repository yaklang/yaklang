#include <stdio.h>

// Test macro constants used in string concatenation
// Error case: "version " FFMPEG_VERSION should be recognized

#define VERSION_MAJOR 1
#define VERSION_MINOR 0
#define VERSION_PATCH 0

#define VERSION_STRING "1.0.0"
#define BUILD_DATE "2024-01-01"

// Error case 1: Macro constant in string (after preprocessing)
int main() {
    printf("version " VERSION_STRING "\n");
    printf("build date: " BUILD_DATE "\n");
    
    // Error case 2: Multiple macro constants
    printf("version " VERSION_STRING " built on " BUILD_DATE "\n");
    
    // Error case 3: In function calls
    char buffer[256];
    snprintf(buffer, sizeof(buffer), "version " VERSION_STRING);
    
    return 0;
}

