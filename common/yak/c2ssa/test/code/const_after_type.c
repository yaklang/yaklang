#include <stdio.h>

// Test const qualifier after type name (C allows both positions)
struct AVCodecDescriptor {
    int id;
    const char *name;
};

int main() {
    // const after type name - the issue from ffmpeg_opt.c
    struct AVCodecDescriptor const *input_descriptor = NULL;
    struct AVCodecDescriptor const *output_descriptor = NULL;
    
    // const before type name (should also work)
    const struct AVCodecDescriptor *input2 = NULL;
    const struct AVCodecDescriptor *output2 = NULL;
    
    // Pointer to const
    int const *ptr1;
    const int *ptr2;
    int * const ptr3 = NULL;  // const pointer
    const int * const ptr4 = NULL;  // const pointer to const
    
    // Function returning const pointer
    const char *get_name(void);
    
    return 0;
}

