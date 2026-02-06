#include <stdio.h>

// Test static variable initialization - the issue from ffmpeg.c
static int run_as_daemon = 0;
static int nb_frames_dup = 0;
static unsigned dup_warning = 1000;
static int nb_frames_drop = 0;
static int64_t decode_error_stat[2] = {0, 0};
static unsigned nb_output_dumped = 0;

// More static initializations
static char *name = "test";
static int array[5] = {1, 2, 3, 4, 5};
static struct {
    int x;
    int y;
} point = {10, 20};

int main() {
    // Local variable initialization
    int local_var = 100;
    unsigned local_unsigned = 200;
    
    return 0;
}

