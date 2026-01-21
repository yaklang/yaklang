def fib(n):
    if n <= 2:
        return 1
    return fib(n-1) + fib(n-2)

def factorial(n):
    result = 1
    i = 1
    while i <= n:
        result = result * i
        i = i + 1
    return result

def sumRange(start, end):
    sum_val = 0
    i = start
    while i <= end:
        sum_val = sum_val + i
        i = i + 1
    return sum_val

def check():
    a = fib(7)
    b = factorial(5)
    c = sumRange(1, 10)
    return a + b + c
