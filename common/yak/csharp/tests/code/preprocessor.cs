#define DEBUG
using System;

public class Preproc
{
    public static int Value()
    {
#if DEBUG
        int x = 1;
#else
        int x = 2;
#endif
        return x;
    }
}
