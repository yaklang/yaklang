import cats.effect.{IO => CIO, _}

@SuppressWarnings(Array("unused"))
class Annotated private (val value: Int) {
  private[this] val cached = value

  def run(): CIO[Int] = CIO.pure(cached)
}
