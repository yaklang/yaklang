/** Layout and self-type regression fixture. */
trait LayoutSupport extends AnyRef {
  self: AnyRef =>

  private[this] val separators: Set[Char] = Set('\u0020', '\u007E')

  protected def describe(value: String): String =
    if (value.isEmpty)
      "empty"
    else value

  def selected(values: List[String]): List[String] =
    for (value <- values if value.nonEmpty)
      yield value.toUpperCase
}