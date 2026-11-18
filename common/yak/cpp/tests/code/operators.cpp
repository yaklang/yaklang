struct Money {
    long cents;
    Money operator+(const Money& o) const { return Money{cents + o.cents}; }
    Money& operator+=(const Money& o) { cents += o.cents; return *this; }
    bool operator==(const Money& o) const { return cents == o.cents; }
    explicit operator bool() const { return cents != 0; }
    Money operator-() const { return Money{-cents}; }
    long operator[](int i) const { return cents + i; }
    void operator()(int x) { cents += x; }
    operator long() const { return cents; }
};
Money operator""_usd(unsigned long long v) { return Money{(long)v}; }
long double operator""_deg(long double d) { return d * 3.14159 / 180; }
int main() {
    Money m{100};
    auto x = m + Money{2} + 3_usd + operator""_usd(7ULL);
    return (int)x + (int)m[1];
}
