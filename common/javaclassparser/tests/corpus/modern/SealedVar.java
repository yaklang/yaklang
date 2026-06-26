public class SealedVar {
    sealed interface Expr permits Num, Add {
    }

    record Num(int value) implements Expr {
    }

    record Add(Expr left, Expr right) implements Expr {
    }

    public int eval(Expr e) {
        if (e instanceof Num n) {
            return n.value();
        } else if (e instanceof Add a) {
            return eval(a.left()) + eval(a.right());
        }
        return 0;
    }

    public int varInference() {
        var list = new java.util.ArrayList<Integer>();
        list.add(1);
        var total = 0;
        for (var x : list) {
            total += x;
        }
        return total;
    }
}
