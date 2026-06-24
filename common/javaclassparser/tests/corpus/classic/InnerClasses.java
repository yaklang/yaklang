public class InnerClasses {
    private int state = 42;

    static class StaticNested {
        int value;

        StaticNested(int v) {
            this.value = v;
        }
    }

    class Inner {
        int read() {
            return state;
        }
    }

    public Runnable makeAnonymous(final int delta) {
        return new Runnable() {
            @Override
            public void run() {
                state += delta;
            }
        };
    }

    public int localClass(int base) {
        class Local {
            int compute() {
                return base * 2 + state;
            }
        }
        return new Local().compute();
    }

    public int useInner() {
        Inner i = new Inner();
        return i.read() + new StaticNested(1).value;
    }
}
