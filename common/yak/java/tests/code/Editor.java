public class Editor {
    private static final String TAG = "Editor";
    private static final boolean DEBUG_UNDO = false;

    // Specifies whether to use the magnifier when pressing the insertion or selection handles.
    private static final boolean FLAG_USE_MAGNIFIER = true;

    /**
     * Returns the X offset to make the pointy top of the error point
     * at the middle of the error icon.
     */
    private int getErrorX() {
        /*
         * The "25" is the distance between the point and the right edge
         * of the background
         */
        final float scale = mTextView.getResources().getDisplayMetrics().density;

        final Drawables dr = mTextView.mDrawables;

        final int layoutDirection = mTextView.getLayoutDirection();
        int errorX;
        int offset;
        switch (layoutDirection) {
            default:
            case View.LAYOUT_DIRECTION_LTR:
                offset = -(dr != null ? dr.mDrawableSizeRight : 0) / 2 + (int) (25 * scale + 0.5f);
                errorX = mTextView.getWidth() - mErrorPopup.getWidth()
                        - mTextView.getPaddingRight() + offset;
                break;
            case View.LAYOUT_DIRECTION_RTL:
                offset = (dr != null ? dr.mDrawableSizeLeft : 0) / 2 - (int) (25 * scale + 0.5f);
                errorX = mTextView.getPaddingLeft() + offset;
                break;
        }
        return errorX;
    }

    /**
     * @hide
     */
    @NonNull public static @DigestEnum
            AlgorithmParameterSpec fromKeymasterToMGF1ParameterSpec(int digest) {
        switch (digest) {
            default:
            case KeymasterDefs.KM_DIGEST_SHA1:
                return MGF1ParameterSpec.SHA1;
            case KeymasterDefs.KM_DIGEST_SHA_2_224:
                return MGF1ParameterSpec.SHA224;
            case KeymasterDefs.KM_DIGEST_SHA_2_256:
                return MGF1ParameterSpec.SHA256;
            case KeymasterDefs.KM_DIGEST_SHA_2_384:
                return MGF1ParameterSpec.SHA384;
            case KeymasterDefs.KM_DIGEST_SHA_2_512:
                return MGF1ParameterSpec.SHA512;
        }
    }

}