//! Inner-documentation regression fixture.
#![allow(
    clippy::needless_return,
)]

trait Fallible {
    type Error: std::error::Error;
}

fn take(value: Option<i32>) -> i32 {
    let Some(v) = value else {
        return 0;
    };
    v
}
