#define FEATURE_A
#undef FEATURE_B

using System;

public class PreprocElif
{
    public static int Pick()
    {
#if FEATURE_B
        int skippedB = 111;
        return skippedB;
#elif FEATURE_A
        int keptA = 222;
        return keptA;
#else
        int skippedElse = 333;
        return skippedElse;
#endif
    }

    public static int Nested()
    {
#if false
        int deadInner = 1;
#if FEATURE_A
        int alsoDead = 2;
#endif
#else
        int liveInner = 3;
        return liveInner;
#endif
    }
}
