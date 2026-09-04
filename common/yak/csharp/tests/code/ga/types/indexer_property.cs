namespace Ga.Types {
    public class GaTypesGrid {
        private int[] _cells = new int[4];
        public int Width { get { return 2; } set { } }
        public int this[int i] {
            get { return _cells[i]; }
            set { _cells[i] = value; }
        }
    }
}
