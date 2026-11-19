local function add(a, b)
    return a + b
end

local t = { 1, 2, 3 }
local i = 0
for k, v in ipairs(t) do
    i = i + v
end
while i < 10 do
    i = i + 1
end
print(add(i, t[1]))
