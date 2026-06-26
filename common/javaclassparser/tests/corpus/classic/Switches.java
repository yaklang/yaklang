public class Switches {
    public int intSwitch(int n) {
        switch (n) {
            case 1:
                return 10;
            case 2:
            case 3:
                return 23;
            case 4: {
                int x = n * 2;
                return x;
            }
            default:
                return -1;
        }
    }

    public int fallthrough(int n) {
        int r = 0;
        switch (n) {
            case 0:
                r += 1;
            case 1:
                r += 2;
            case 2:
                r += 4;
                break;
            default:
                r = -1;
        }
        return r;
    }

    public String stringSwitch(String s) {
        switch (s) {
            case "a":
                return "alpha";
            case "b":
                return "beta";
            default:
                return "other";
        }
    }

    public int charSwitch(char c) {
        switch (c) {
            case 'x':
                return 1;
            case 'y':
                return 2;
            default:
                return 0;
        }
    }
}
