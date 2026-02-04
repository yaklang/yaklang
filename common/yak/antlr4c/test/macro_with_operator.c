// Test macro definitions with operators as parameters
// Error case: FUN(fmin, double, <) - macro parameter contains operator

#define FUN(name, type, op) \
    type name(type x, type y) { \
        return x op y ? x : y; \
    }

// This should generate functions like:
// double fmin(double x, double y) { return x < y ? x : y; }
// double fmax(double x, double y) { return x > y ? x : y; }

FUN(fmin, double, <)
FUN(fmax, double, >)
FUN(fminf, float, <)
FUN(fmaxf, float, >)

int main() {
    double a = 10.5;
    double b = 20.3;
    double result1 = fmin(a, b);
    double result2 = fmax(a, b);
    return 0;
}

