namespace Ga.Control {
    public class GaControlChecked {
        public static int Add(int a, int b) {
            int x = checked(a + b);
            int y = unchecked(a + b);
            checked { x = x + 1; }
            unchecked { y = y + 1; }
            return x + y;
        }
    }
}
