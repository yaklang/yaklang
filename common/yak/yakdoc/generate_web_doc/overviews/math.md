`math` 库是 Go 标准库 `math` 的 yak 封装，提供常用数学函数：三角函数、幂与开方、取整、绝对值与 NaN 判断等，用于数值计算场景。

典型使用场景：

- 基础运算：`math.Abs` / `math.Pow` / `math.Pow10` / `math.Sqrt`。
- 取整：`math.Ceil` / `math.Floor` / `math.Round` / `math.RoundToEven`。
- 三角与特殊值：`math.Sin` / `math.Cos` / `math.Tan` / `math.Asin` / `math.Acos` / `math.Atan` / `math.Sinh`，`math.NaN` / `math.IsNaN` 处理非数值。

与相邻库的关系：`math` 是纯计算库、无副作用，常与 `str`（数值转换）、统计/打分类逻辑配合使用。
