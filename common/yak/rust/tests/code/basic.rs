mod util;

fn add(a: i32, b: i32) -> i32 {
    let c = a + b;
    return c;
}

fn main() {
    let mut x = 1;
    x = add(x, 2);
    if x > 2 {
        x = x - 1;
    } else {
        x = 0;
    }
    while x < 10 {
        x = x + 1;
    }
    for i in 0..3 {
        x = x + i;
    }
    println!("{}", x);
}
