package codec;

// Reproduces Bug AL's switch-case "declared inside case, read after switch" shape (fastjson2
// DateUtils.parseLocalDateTime hand-unrolled parser: each pattern case copies the canonical date/time
// digit chars into a fresh set of locals, which are then validated AFTER the switch). In bytecode the
// locals are slots first written inside each case and read past the switch; javac's definite-assignment
// is satisfied because every case (incl. default) assigns them on the taken path. The decompiler mints
// a distinct id per case for each slot (rendered varN, colliding by name) while the post-switch read
// keeps the slot's original id, so switchHoistDeclarations' identity-based "read after switch" probe
// misses and the in-case `int a = ...` declarations stay scoped to their case -> the post-switch read
// is out of scope ("cannot find symbol: variable a"). The fix probes "read after switch" by the slot's
// stable VarUid (same logical variable) so the declaration is hoisted ahead of the switch and the
// per-case stores are demoted to plain assignments.
public class SwitchCaseLocalReadAfter {
    static int classify(int sel, char[] cs) {
        int a;
        int b;
        int c;
        switch (sel) {
            case 1:
                if (cs[0] == '-') {
                    a = cs[1];
                    b = cs[2];
                    c = cs[3];
                } else {
                    a = 0;
                    b = 0;
                    c = 0;
                }
                break;
            case 2:
                if (cs[0] == '/') {
                    a = cs[4];
                    b = cs[5];
                    c = cs[6];
                } else {
                    a = 1;
                    b = 1;
                    c = 1;
                }
                break;
            default:
                a = 9;
                b = 9;
                c = 9;
        }
        int sum = 0;
        if (a >= '0' && a <= '9') {
            sum += a - '0';
        }
        if (b >= '0' && b <= '9') {
            sum += (b - '0') * 10;
        }
        if (c >= '0' && c <= '9') {
            sum += (c - '0') * 100;
        }
        return sum;
    }

    public static void main(String[] args) {
        char[] cs = "-1234567".toCharArray();
        StringBuilder sb = new StringBuilder();
        sb.append(classify(1, cs)).append(';');
        sb.append(classify(2, "/4567890".toCharArray())).append(';');
        sb.append(classify(7, cs)).append(';');
        System.out.println(sb.toString());
    }
}
