public class Inheritance {
    interface Shape {
        double area();

        default String describe() {
            return "shape with area " + area();
        }
    }

    static abstract class Base implements Shape {
        protected String name;

        Base(String name) {
            this.name = name;
        }

        public abstract double area();

        public String getName() {
            return name;
        }
    }

    static class Circle extends Base {
        private double r;

        Circle(double r) {
            super("circle");
            this.r = r;
        }

        @Override
        public double area() {
            return Math.PI * r * r;
        }

        @Override
        public String describe() {
            return super.describe() + " (" + getName() + ")";
        }
    }

    public String run() {
        Shape s = new Circle(2.0);
        return s.describe();
    }
}
