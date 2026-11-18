local function add(a, b)
    local c = a + b
    return c
end

function main()
    local x = 1
    x = add(x, 2)
    if x > 2 then
        x = x - 1
    else
        x = 0
    end
    while x < 10 do
        x = x + 1
    end
    for i = 1, 3 do
        x = x + i
    end
    println(x)
end

main()
