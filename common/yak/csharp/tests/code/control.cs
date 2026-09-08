using System;

public class Control
{
    public static int Run(int n)
    {
        int acc = 0;
        if (n > 0)
        {
            acc = acc + 1;
        }
        else
        {
            acc = acc - 1;
        }

        for (int i = 0; i < n; i++)
        {
            acc = acc + i;
        }

        int k = 0;
        while (k < 3)
        {
            k++;
        }

        foreach (var x in new int[] {1, 2, 3})
        {
            acc = acc + x;
        }

        switch (n)
        {
            case 1:
                acc = 10;
                break;
            default:
                acc = 20;
                break;
        }

        try
        {
            acc = acc + 1;
        }
        catch (Exception ex)
        {
            acc = 0;
        }
        finally
        {
            acc = acc;
        }
        return acc;
    }
}
