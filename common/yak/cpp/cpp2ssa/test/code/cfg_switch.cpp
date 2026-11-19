// Control-flow fixtures for switch statements.

int selectCase(int value) {
    int result = 0;
    switch (value) {
    case 1:
        result = 10;
        break;
    case 2:
        result = 20;
        break;
    default:
        result = -1;
        break;
    }
    return result;
}

int fallThrough(int value) {
    int result = 0;
    switch (value) {
    case 1:
    case 2:
        result = 12;
        break;
    case 3:
        result = 3;
        break;
    }
    return result;
}