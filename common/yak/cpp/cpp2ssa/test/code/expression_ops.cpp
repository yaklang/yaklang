// Expression fixtures: operands and operators the SSA records.

int arithmetic(int a, int b) {
    int sum = a + b;
    int difference = a - b;
    int product = a * b;
    int quotient = a / b;
    int remainder = a % b;
    return sum + difference + product + quotient + remainder;
}

int comparison(int a, int b) {
    int score = 0;
    if (a == b) {
        score += 1;
    }
    if (a != b) {
        score += 2;
    }
    if (a < b) {
        score += 4;
    }
    if (a <= b) {
        score += 8;
    }
    if (a > b) {
        score += 16;
    }
    if (a >= b) {
        score += 32;
    }
    return score;
}

int logical(int a, int b) {
    int score = 0;
    if (a > 0 && b > 0) {
        score = 1;
    }
    if (a > 0 || b > 0) {
        score += 2;
    }
    if (!a) {
        score += 4;
    }
    return score;
}

int bitwise(int a, int b) {
    return (a & b) | (a ^ b) | (a << 1) | (a >> 1);
}

int compound(int a) {
    int value = a;
    value += 2;
    value -= 1;
    value *= 3;
    value /= 2;
    return value;
}

int increments(int a) {
    int value = a;
    value++;
    ++value;
    value--;
    return value;
}

int casts(int a) {
    int widened = static_cast<int>(a);
    return widened + (int)a;
}

int condition(int a, int b) {
    int picked = a > b ? a : b;
    return picked;
}

int strings() {
    return 0;
}

int literals() {
    int decimal = 42;
    int hex = 0x2A;
    int binary = 0b1010;
    double ratio = 1.5;
    char letter = 'x';
    bool flag = true;
    return decimal + hex + binary + (int)ratio + letter + (flag ? 1 : 0);
}

int sizes() {
    return sizeof(int) + sizeof(char);
}