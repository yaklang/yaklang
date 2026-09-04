namespace Ga.Control {
    public class GaControlLoops {
        public static int Acc(int n) {
            int a = 0;
            for (int i = 0; i < n; i++) a += i;
            int k = 0;
            while (k < 3) { k++; a += k; }
            do { k--; } while (k > 0);
            foreach (var x in new int[] { 1, 2, 3 }) a += x;
            return a;
        }
    }
}
