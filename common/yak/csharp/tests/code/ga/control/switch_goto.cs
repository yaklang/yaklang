namespace Ga.Control {
    public class GaControlSwitchGoto {
        public static int Run(int n) {
            switch (n) {
                case 0: goto case 1;
                case 1: return 10;
                default: goto End;
            }
        End:
            return 20;
        }
    }
}
