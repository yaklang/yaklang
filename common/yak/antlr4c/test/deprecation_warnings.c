// Test case for FF_DISABLE_DEPRECATION_WARNINGS and FF_ENABLE_DEPRECATION_WARNINGS macros
// This pattern is common in FFmpeg code
// Note: In real code, these macros are typically used inside functions

void test_function() {
#if FF_API_CODED_FRAME
FF_DISABLE_DEPRECATION_WARNINGS
    avctx->coded_frame->pict_type = AV_PICTURE_TYPE_I;
    avctx->coded_frame->key_frame = 1;
FF_ENABLE_DEPRECATION_WARNINGS
#endif
}

// Another variation with multiple statements
void another_function() {
#if SOME_FEATURE
FF_DISABLE_DEPRECATION_WARNINGS
    obj->member->field = value;
    another->member = 42;
FF_ENABLE_DEPRECATION_WARNINGS
#endif
}

// Nested case
void nested_function() {
#if COND1
FF_DISABLE_DEPRECATION_WARNINGS
    if (condition) {
        obj->field = value;
    }
FF_ENABLE_DEPRECATION_WARNINGS
#endif
}
