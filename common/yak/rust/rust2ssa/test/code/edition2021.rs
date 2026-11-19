async fn fetch(x: i32) -> i32 {
    x + 1
}

fn main() {
    let values = [1, 2, 3];
    for value in values {
        println!("{}", value);
    }
    let result = fetch(1);
    println!("{}", result);
}
