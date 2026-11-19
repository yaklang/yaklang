-- Regression: empty line comments must not be lexed as '--'.
--
--[[ A block comment must win over the line-comment rule. ]]
local function add(a, b)
    return a + b
end

for i = 1, 10 do
    if i % 2 == 0 then
        goto continue
    end
    print(i)
    ::continue::
end

local mask = 0x01ULL
return add(mask, 1);
