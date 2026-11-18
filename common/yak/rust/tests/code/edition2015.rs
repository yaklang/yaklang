extern crate core;
use std::collections::HashMap;

fn main() {
    let mut counts = HashMap::new();
    counts.insert("a", 1);
    let value = counts.get("a");
    println!("{:?}", value);
}
