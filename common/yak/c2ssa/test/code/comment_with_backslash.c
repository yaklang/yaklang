// Test case for comments with backslash line continuation
// This pattern is common in C code

void test_function() {
    int x = 0;
    /* Collapse any two states with the same previous sample value. \
     * One could also distinguish states by step and by 2nd to last
     * sample, but the effects of that are negligible.
     * Since nodes in the previous generation are iterated
     * through a heap, they're roughly ordered from better to
     * worse, but not strictly ordered. Therefore, an earlier
     * node with the same sample value is better in most cases
     */
    x = 1;
}

