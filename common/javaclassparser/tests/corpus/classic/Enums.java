public class Enums {
    enum Color {
        RED, GREEN, BLUE
    }

    enum Planet {
        MERCURY(3.303e+23, 2.4397e6),
        EARTH(5.976e+24, 6.37814e6);

        private final double mass;
        private final double radius;

        Planet(double mass, double radius) {
            this.mass = mass;
            this.radius = radius;
        }

        double gravity() {
            return 6.67300E-11 * mass / (radius * radius);
        }
    }

    public String describe(Color c) {
        switch (c) {
            case RED:
                return "warm";
            case GREEN:
                return "cool";
            case BLUE:
                return "cold";
            default:
                return "unknown";
        }
    }

    public double earthGravity() {
        return Planet.EARTH.gravity();
    }

    public Color[] all() {
        return Color.values();
    }
}
