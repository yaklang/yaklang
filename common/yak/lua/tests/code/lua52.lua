local function count(n)
    local i = 0
    ::again::
    i = i + 1
    if i < n then
        goto again
    end
    return i
end

print(count(3))
