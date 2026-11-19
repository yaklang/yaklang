// Control-flow fixtures for loops and their Phi inputs.

int sumFor(int n) {
    int total = 0;
    for (int i = 0; i < n; i++) {
        total += i;
    }
    return total;
}

int sumWhile(int n) {
    int index = 0;
    int total = 0;
    while (index < n) {
        total += index;
        index++;
    }
    return total;
}

int sumDoWhile(int n) {
    int total = 0;
    do {
        total++;
    } while (total < n);
    return total;
}

int sumRange(int n) {
    int total = 0;
    int values[3] = {1, 2, 3};
    for (int value : values) {
        total += value;
    }
    return total;
}

int sumWithBreak(int n) {
    int total = 0;
    for (int i = 0; i < n; i++) {
        if (i == 4) {
            break;
        }
        if (i % 2 == 0) {
            continue;
        }
        total += i;
    }
    return total;
}