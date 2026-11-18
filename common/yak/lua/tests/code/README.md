# Lua syntax fixtures

All files in this directory are original fixtures written for the yaklang Lua
frontend. They are licensed under the repository license.

`regression_comment_label.lua` records constructs observed while parsing real
projects, but no source code was copied:

- `--` empty comment lines and `--[[ ... ]]` block comments, observed in
  [Kong](https://github.com/Kong/kong) (Apache-2.0).
- `goto continue` / `::continue::` loop labels, observed in Kong (Apache-2.0).
- The LuaJIT `0x01ULL` integer suffix, observed in Kong's router code
  (Apache-2.0). The lexer accepts the suffix; it is not evaluated as a 64-bit
  cdata value.

Real project checkouts are not committed. The syntax fixtures run through
`TestAllSyntaxForLua_G4` in the syntax-g4 workflow.
