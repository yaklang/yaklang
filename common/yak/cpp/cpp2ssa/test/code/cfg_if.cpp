// Control-flow fixtures for the SSA if/else merge points.

int choose(int a, int b) {
    int result = 0;
    if (a > b) {
        result = a;
    } else {
        result = b;
    }
    return result;
}

int chooseDefault(int a) {
    int result = 7;
    if (a > 0) {
        result = 1;
    }
    return result;
}

int classify(int value) {
    int label = 0;
    if (value < 0) {
        label = -1;
    } else if (value == 0) {
        label = 0;
    } else {
        label = 1;
    }
    return label;
}

int nested(int a, int b) {
    int result = 0;
    if (a > 0) {
        if (b > 0) {
            result = 11;
        } else {
            result = 12;
        }
    } else {
        result = 13;
    }
    return result;
}