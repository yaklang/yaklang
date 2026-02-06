// Test case for macro calls followed by array declarations
// This pattern is common in FFmpeg code

// Example: DECLARE_ALIGNED(SBC_ALIGN, int32_t, ff_sbcdsp_joint_bits_mask)[8]
DECLARE_ALIGNED(SBC_ALIGN, int32_t, ff_sbcdsp_joint_bits_mask)[8] = {
    8,   4,  2,  1, 128, 64, 32, 16
};

// Another example with different alignment
DECLARE_ALIGNED(32, float, spec1)[256];

// With initialization
DECLARE_ALIGNED(16, int16_t, buffer)[1024] = {0};

// Multiple dimensions
DECLARE_ALIGNED(64, double, matrix)[10][20];

