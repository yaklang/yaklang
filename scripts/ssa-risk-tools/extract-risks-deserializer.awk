#!/usr/bin/awk -f

# extract-risks-deserializer.awk
# 增强版JSON反序列化AWK脚本，支持完整的JSON结构解析
# 用法: awk -f extract-risks-deserializer.awk risk.json

BEGIN {
    # 设置字段分隔符
    FS = ""
    
    # 初始化变量
    in_risks = 0
    in_risk = 0
    risk_count = 0
    current_risk = ""
    
    # 风险数据结构
    risk_data["file"] = ""
    risk_data["line"] = 0
    risk_data["severity"] = ""
    risk_data["title"] = ""
    risk_data["title_verbose"] = ""
    risk_data["description"] = ""
    risk_data["solution"] = ""
    risk_data["rule_name"] = ""
    risk_data["function_name"] = ""
    risk_data["program_name"] = ""
    risk_data["language"] = ""
    risk_data["risk_type"] = ""
    risk_data["cve"] = ""
    risk_data["cwe"] = ""
    risk_data["time"] = ""
    risk_data["latest_disposal_status"] = ""
    risk_data["code_range"] = ""
    risk_data["start_line"] = 0
    risk_data["end_line"] = 0
    risk_data["start_column"] = 0
    risk_data["end_column"] = 0
    risk_data["code_fragment"] = ""
    
    # 多行字段处理变量
    in_description = 0
    in_solution = 0
    in_code_fragment = 0
    description_raw = ""
    solution_raw = ""
    code_fragment_raw = ""
    
    # JSON解析状态
    json_depth = 0
    in_string = 0
    escaped = 0
    current_key = ""
    current_value = ""
    
    # 创建 results 目录
    results_dir = "results"
    system("mkdir -p " results_dir)
}

# 通用JSON字符串解析函数
function parse_json_string(json_str, result) {
    # 移除首尾引号
    if (substr(json_str, 1, 1) == "\"" && substr(json_str, length(json_str), 1) == "\"") {
        json_str = substr(json_str, 2, length(json_str) - 2)
    }
    
    # 处理转义字符
    result = json_str
    gsub(/\\r\\n/, "\n", result)  # Windows换行
    gsub(/\\n/, "\n", result)     # Unix换行
    gsub(/\\t/, "\t", result)     # 制表符
    gsub(/\\"/, "\"", result)     # 引号
    gsub(/\\\\/, "\\", result)    # 反斜杠
    gsub(/\\u([0-9a-fA-F]{4})/, "\\u\\1", result)  # Unicode转义
    
    return result
}

# JSON对象解析函数
function parse_json_object(obj_str, result_obj) {
    # 移除首尾大括号
    if (substr(obj_str, 1, 1) == "{" && substr(obj_str, length(obj_str), 1) == "}") {
        obj_str = substr(obj_str, 2, length(obj_str) - 2)
    }
    
    # 按逗号分割键值对
    split(obj_str, pairs, ",")
    
    for (i = 1; i <= length(pairs); i++) {
        pair = pairs[i]
        # 查找冒号位置
        colon_pos = index(pair, ":")
        if (colon_pos > 0) {
            key = substr(pair, 1, colon_pos - 1)
            value = substr(pair, colon_pos + 1)
            
            # 清理键名（移除引号和空格）
            gsub(/^[[:space:]]*"[[:space:]]*/, "", key)
            gsub(/[[:space:]]*"[[:space:]]*$/, "", key)
            
            # 清理值（移除引号和空格）
            gsub(/^[[:space:]]*"[[:space:]]*/, "", value)
            gsub(/[[:space:]]*"[[:space:]]*$/, "", value)
            
            result_obj[key] = value
        }
    }
}

# JSON数组解析函数
function parse_json_array(array_str, result_array) {
    # 移除首尾方括号
    if (substr(array_str, 1, 1) == "[" && substr(array_str, length(array_str), 1) == "]") {
        array_str = substr(array_str, 2, length(array_str) - 2)
    }
    
    # 按逗号分割数组元素
    split(array_str, elements, ",")
    
    for (i = 1; i <= length(elements); i++) {
        element = elements[i]
        # 清理元素（移除引号和空格）
        gsub(/^[[:space:]]*"[[:space:]]*/, "", element)
        gsub(/[[:space:]]*"[[:space:]]*$/, "", element)
        result_array[i] = element
    }
}

# 数字解析函数
function parse_json_number(num_str) {
    # 移除空格
    gsub(/^[[:space:]]+|[[:space:]]+$/, "", num_str)
    
    # 检查是否为整数
    if (match(num_str, /^-?[0-9]+$/)) {
        return int(num_str)
    }
    # 检查是否为浮点数
    else if (match(num_str, /^-?[0-9]+\.[0-9]+$/)) {
        return num_str + 0  # AWK自动转换为数字
    }
    else {
        return 0
    }
}

# 布尔值解析函数
function parse_json_boolean(bool_str) {
    gsub(/^[[:space:]]+|[[:space:]]+$/, "", bool_str)
    if (bool_str == "true") return 1
    if (bool_str == "false") return 0
    if (bool_str == "null") return ""
    return bool_str
}

# 检测到Risks字段开始
/"Risks"\s*:\s*\{/ {
    in_risks = 1
    next
}

# 检测到Risks字段结束
in_risks && /^\s*\}\s*,?\s*$/ {
    in_risks = 0
    next
}

# 在Risks字段内，检测到新的风险项
in_risks && /"[a-f0-9]{40}"\s*:\s*\{/ {
    # 如果有之前的风险，先输出
    if (current_risk != "") {
        output_risk()
    }
    
    # 开始新的风险
    in_risk = 1
    current_risk = ""
    
    # 重置所有风险数据
    for (key in risk_data) {
        risk_data[key] = ""
    }
    risk_data["line"] = 0
    risk_data["start_line"] = 0
    risk_data["end_line"] = 0
    risk_data["start_column"] = 0
    risk_data["end_column"] = 0
    
    # 重置多行字段处理变量
    in_description = 0
    in_solution = 0
    in_code_fragment = 0
    description_raw = ""
    solution_raw = ""
    code_fragment_raw = ""
    
    # 提取风险ID (40位哈希值)
    match($0, /"([a-f0-9]{40})"[[:space:]]*:[[:space:]]*{/, arr)
    current_risk = arr[1]
    next
}

# 在风险项内，检测到code_source_url字段 (对应文件路径)
in_risk && /"code_source_url"[[:space:]]*:[[:space:]]*"[^"]*"/ {
    match($0, /"code_source_url"[[:space:]]*:[[:space:]]*"([^"]*)"/, arr)
    risk_data["file"] = arr[1]
    next
}

# 检测line字段
in_risk && /"line"[[:space:]]*:[[:space:]]*[0-9]+/ {
    match($0, /"line"[[:space:]]*:[[:space:]]*([0-9]+)/, arr)
    risk_data["line"] = arr[1]
    next
}

# 检测severity字段
in_risk && /"severity"[[:space:]]*:[[:space:]]*"[^"]*"/ {
    match($0, /"severity"[[:space:]]*:[[:space:]]*"([^"]*)"/, arr)
    risk_data["severity"] = arr[1]
    next
}

# 检测title字段
in_risk && /"title"[[:space:]]*:[[:space:]]*"[^"]*"/ {
    match($0, /"title"[[:space:]]*:[[:space:]]*"([^"]*)"/, arr)
    risk_data["title"] = arr[1]
    next
}

# 检测title_verbose字段
in_risk && /"title_verbose"[[:space:]]*:[[:space:]]*"[^"]*"/ {
    match($0, /"title_verbose"[[:space:]]*:[[:space:]]*"([^"]*)"/, arr)
    risk_data["title_verbose"] = arr[1]
    next
}

# 检测rule_name字段
in_risk && /"rule_name"[[:space:]]*:[[:space:]]*"[^"]*"/ {
    match($0, /"rule_name"[[:space:]]*:[[:space:]]*"([^"]*)"/, arr)
    risk_data["rule_name"] = arr[1]
    next
}

# 检测function_name字段
in_risk && /"function_name"[[:space:]]*:[[:space:]]*"[^"]*"/ {
    match($0, /"function_name"[[:space:]]*:[[:space:]]*"([^"]*)"/, arr)
    risk_data["function_name"] = arr[1]
    next
}

# 检测program_name字段
in_risk && /"program_name"[[:space:]]*:[[:space:]]*"[^"]*"/ {
    match($0, /"program_name"[[:space:]]*:[[:space:]]*"([^"]*)"/, arr)
    risk_data["program_name"] = arr[1]
    next
}

# 检测language字段
in_risk && /"language"[[:space:]]*:[[:space:]]*"[^"]*"/ {
    match($0, /"language"[[:space:]]*:[[:space:]]*"([^"]*)"/, arr)
    risk_data["language"] = arr[1]
    next
}

# 检测risk_type字段
in_risk && /"risk_type"[[:space:]]*:[[:space:]]*"[^"]*"/ {
    match($0, /"risk_type"[[:space:]]*:[[:space:]]*"([^"]*)"/, arr)
    risk_data["risk_type"] = arr[1]
    next
}

# 检测cve字段
in_risk && /"cve"[[:space:]]*:[[:space:]]*"[^"]*"/ {
    match($0, /"cve"[[:space:]]*:[[:space:]]*"([^"]*)"/, arr)
    risk_data["cve"] = arr[1]
    next
}

# 检测cwe字段 (数组格式)
in_risk && /"cwe"[[:space:]]*:[[:space:]]*\[/ {
    # 简单处理，提取数组内容
    match($0, /"cwe"[[:space:]]*:[[:space:]]*\[([^\]]*)\]/, arr)
    risk_data["cwe"] = arr[1]
    next
}

# 检测time字段
in_risk && /"time"[[:space:]]*:[[:space:]]*"[^"]*"/ {
    match($0, /"time"[[:space:]]*:[[:space:]]*"([^"]*)"/, arr)
    risk_data["time"] = arr[1]
    next
}

# 检测latest_disposal_status字段
in_risk && /"latest_disposal_status"[[:space:]]*:[[:space:]]*"[^"]*"/ {
    match($0, /"latest_disposal_status"[[:space:]]*:[[:space:]]*"([^"]*)"/, arr)
    risk_data["latest_disposal_status"] = arr[1]
    next
}

# 检测description字段
in_risk && /"description"[[:space:]]*:[[:space:]]*"/ {
    # 定位到 description 字段
    start_pos = index($0, "description")
    if (start_pos > 0) {
        # 找到冒号位置
        colon_pos = index(substr($0, start_pos), ":")
        if (colon_pos > 0) {
            # 找到第一个引号
            quote_start = index(substr($0, start_pos + colon_pos), "\"")
            if (quote_start > 0) {
                # 提取从第一个引号开始到行尾的内容
                rest_line = substr($0, start_pos + colon_pos + quote_start)
                
                # 检查是否是单行完整的 JSON 字符串
                # 寻找真正的结束引号（跳过转义的引号）
                end_quote_pos = 0
                escaped = 0
                for (i = 2; i <= length(rest_line); i++) {
                    char = substr(rest_line, i, 1)
                    if (escaped) {
                        escaped = 0
                    } else if (char == "\\") {
                        escaped = 1
                    } else if (char == "\"") {
                        end_quote_pos = i
                        break
                    }
                }
                
                if (end_quote_pos > 0) {
                    # 单行完整的 JSON 字符串
                    description_raw = substr(rest_line, 2, end_quote_pos - 2)
                } else {
                    # 多行 JSON 字符串，需要继续读取后续行
                    description_raw = substr(rest_line, 2)  # 去掉第一个引号
                    in_description = 1
                    description_lines = 1
                }
                
                if (end_quote_pos > 0) {
                    # 处理常见的转义字符
                    risk_data["description"] = description_raw
                    # 替换 \r\n 为换行符（Windows风格换行）
                    gsub(/\\r\\n/, "\n", risk_data["description"])
                    # 替换 \n 为真正的换行符
                    gsub(/\\n/, "\n", risk_data["description"])
                    # 替换 \t 为真正的制表符
                    gsub(/\\t/, "\t", risk_data["description"])
                    # 替换 \" 为 "
                    gsub(/\\"/, "\"", risk_data["description"])
                    # 替换 \\ 为 \
                    gsub(/\\\\/, "\\", risk_data["description"])
                }
            }
        }
    }
    next
}

# 处理多行 description 字段
in_risk && in_description {
    # 寻找结束引号
    end_quote_pos = 0
    escaped = 0
    for (i = 1; i <= length($0); i++) {
        char = substr($0, i, 1)
        if (escaped) {
            escaped = 0
        } else if (char == "\\") {
            escaped = 1
        } else if (char == "\"") {
            end_quote_pos = i
            break
        }
    }
    
    if (end_quote_pos > 0) {
        # 找到结束引号，完成 description 字段的读取
        description_raw = description_raw substr($0, 1, end_quote_pos - 1)
        
        # 处理常见的转义字符
        risk_data["description"] = description_raw
        # 替换 \r\n 为换行符（Windows风格换行）
        gsub(/\\r\\n/, "\n", risk_data["description"])
        # 替换 \n 为真正的换行符
        gsub(/\\n/, "\n", risk_data["description"])
        # 替换 \t 为真正的制表符
        gsub(/\\t/, "\t", risk_data["description"])
        # 替换 \" 为 "
        gsub(/\\"/, "\"", risk_data["description"])
        # 替换 \\ 为 \
        gsub(/\\\\/, "\\", risk_data["description"])
        
        in_description = 0
        description_raw = ""
    } else {
        # 继续累积内容
        description_raw = description_raw $0
    }
    next
}

# 检测solution字段
in_risk && /"solution"[[:space:]]*:[[:space:]]*"/ {
    # 定位到 solution 字段
    start_pos = index($0, "solution")
    if (start_pos > 0) {
        # 找到冒号位置
        colon_pos = index(substr($0, start_pos), ":")
        if (colon_pos > 0) {
            # 找到第一个引号
            quote_start = index(substr($0, start_pos + colon_pos), "\"")
            if (quote_start > 0) {
                # 提取从第一个引号开始到行尾的内容
                rest_line = substr($0, start_pos + colon_pos + quote_start)
                
                # 检查是否是单行完整的 JSON 字符串
                # 寻找真正的结束引号（跳过转义的引号）
                end_quote_pos = 0
                escaped = 0
                for (i = 2; i <= length(rest_line); i++) {
                    char = substr(rest_line, i, 1)
                    if (escaped) {
                        escaped = 0
                    } else if (char == "\\") {
                        escaped = 1
                    } else if (char == "\"") {
                        end_quote_pos = i
                        break
                    }
                }
                
                if (end_quote_pos > 0) {
                    # 单行完整的 JSON 字符串
                    solution_raw = substr(rest_line, 2, end_quote_pos - 2)
                } else {
                    # 多行 JSON 字符串，需要继续读取后续行
                    solution_raw = substr(rest_line, 2)  # 去掉第一个引号
                    in_solution = 1
                    solution_lines = 1
                }
                
                if (end_quote_pos > 0) {
                    # 处理常见的转义字符
                    risk_data["solution"] = solution_raw
                    # 替换 \r\n 为换行符（Windows风格换行）
                    gsub(/\\r\\n/, "\n", risk_data["solution"])
                    # 替换 \n 为真正的换行符
                    gsub(/\\n/, "\n", risk_data["solution"])
                    # 替换 \t 为真正的制表符
                    gsub(/\\t/, "\t", risk_data["solution"])
                    # 替换 \" 为 "
                    gsub(/\\"/, "\"", risk_data["solution"])
                    # 替换 \\ 为 \
                    gsub(/\\\\/, "\\", risk_data["solution"])
                }
            }
        }
    }
    next
}

# 处理多行 solution 字段
in_risk && in_solution {
    # 寻找结束引号
    end_quote_pos = 0
    escaped = 0
    for (i = 1; i <= length($0); i++) {
        char = substr($0, i, 1)
        if (escaped) {
            escaped = 0
        } else if (char == "\\") {
            escaped = 1
        } else if (char == "\"") {
            end_quote_pos = i
            break
        }
    }
    
    if (end_quote_pos > 0) {
        # 找到结束引号，完成 solution 字段的读取
        solution_raw = solution_raw substr($0, 1, end_quote_pos - 1)
        
        # 处理常见的转义字符
        risk_data["solution"] = solution_raw
        # 替换 \r\n 为换行符（Windows风格换行）
        gsub(/\\r\\n/, "\n", risk_data["solution"])
        # 替换 \n 为真正的换行符
        gsub(/\\n/, "\n", risk_data["solution"])
        # 替换 \t 为真正的制表符
        gsub(/\\t/, "\t", risk_data["solution"])
        # 替换 \" 为 "
        gsub(/\\"/, "\"", risk_data["solution"])
        # 替换 \\ 为 \
        gsub(/\\\\/, "\\", risk_data["solution"])
        
        in_solution = 0
        solution_raw = ""
    } else {
        # 继续累积内容
        solution_raw = solution_raw $0
    }
    next
}

# 检测code_fragment字段
in_risk && /"code_fragment"[[:space:]]*:[[:space:]]*"/ {
    # 定位到 code_fragment 字段
    start_pos = index($0, "code_fragment")
    if (start_pos > 0) {
        # 找到冒号位置
        colon_pos = index(substr($0, start_pos), ":")
        if (colon_pos > 0) {
            # 找到第一个引号
            quote_start = index(substr($0, start_pos + colon_pos), "\"")
            if (quote_start > 0) {
                # 提取从第一个引号开始到行尾的内容
                rest_line = substr($0, start_pos + colon_pos + quote_start)
                
                # 检查是否是单行完整的 JSON 字符串
                # 寻找真正的结束引号（跳过转义的引号）
                end_quote_pos = 0
                escaped = 0
                for (i = 2; i <= length(rest_line); i++) {
                    char = substr(rest_line, i, 1)
                    if (escaped) {
                        escaped = 0
                    } else if (char == "\\") {
                        escaped = 1
                    } else if (char == "\"") {
                        end_quote_pos = i
                        break
                    }
                }
                
                if (end_quote_pos > 0) {
                    # 单行完整的 JSON 字符串
                    code_fragment_raw = substr(rest_line, 2, end_quote_pos - 2)
                } else {
                    # 多行 JSON 字符串，需要继续读取后续行
                    code_fragment_raw = substr(rest_line, 2)  # 去掉第一个引号
                    in_code_fragment = 1
                    code_fragment_lines = 1
                }
                
                if (end_quote_pos > 0) {
                    # 处理常见的转义字符
                    risk_data["code_fragment"] = code_fragment_raw
                    # 替换 \r\n 为换行符（Windows风格换行）
                    gsub(/\\r\\n/, "\n", risk_data["code_fragment"])
                    # 替换 \n 为真正的换行符
                    gsub(/\\n/, "\n", risk_data["code_fragment"])
                    # 替换 \t 为真正的制表符
                    gsub(/\\t/, "\t", risk_data["code_fragment"])
                    # 替换 \" 为 "
                    gsub(/\\"/, "\"", risk_data["code_fragment"])
                    # 替换 \\ 为 \
                    gsub(/\\\\/, "\\", risk_data["code_fragment"])
                }
            }
        }
    }
    next
}

# 处理多行 code_fragment 字段
in_risk && in_code_fragment {
    # 寻找结束引号
    end_quote_pos = 0
    escaped = 0
    for (i = 1; i <= length($0); i++) {
        char = substr($0, i, 1)
        if (escaped) {
            escaped = 0
        } else if (char == "\\") {
            escaped = 1
        } else if (char == "\"") {
            end_quote_pos = i
            break
        }
    }
    
    if (end_quote_pos > 0) {
        # 找到结束引号，完成 code_fragment 字段的读取
        code_fragment_raw = code_fragment_raw substr($0, 1, end_quote_pos - 1)
        
        # 处理常见的转义字符
        risk_data["code_fragment"] = code_fragment_raw
        # 替换 \r\n 为换行符（Windows风格换行）
        gsub(/\\r\\n/, "\n", risk_data["code_fragment"])
        # 替换 \n 为真正的换行符
        gsub(/\\n/, "\n", risk_data["code_fragment"])
        # 替换 \t 为真正的制表符
        gsub(/\\t/, "\t", risk_data["code_fragment"])
        # 替换 \" 为 "
        gsub(/\\"/, "\"", risk_data["code_fragment"])
        # 替换 \\ 为 \
        gsub(/\\\\/, "\\", risk_data["code_fragment"])
        
        in_code_fragment = 0
        code_fragment_raw = ""
    } else {
        # 继续累积内容
        code_fragment_raw = code_fragment_raw $0
    }
    next
}

# 检测code_range字段
in_risk && /code_range/ {
    # 定位到 code_range 字段
    start_pos = index($0, "code_range")
    if (start_pos > 0) {
        # 找到冒号位置
        colon_pos = index(substr($0, start_pos), ":")
        if (colon_pos > 0) {
            # 找到第一个引号
            quote_start = index(substr($0, start_pos + colon_pos), "\"")
            if (quote_start > 0) {
                # 找到 JSON 字符串的结束引号
                rest_line = substr($0, start_pos + colon_pos + quote_start)
                
                # 寻找真正的结束引号（跳过转义的引号）
                end_quote_pos = 0
                escaped = 0
                for (i = 2; i <= length(rest_line); i++) {
                    char = substr(rest_line, i, 1)
                    if (escaped) {
                        escaped = 0
                    } else if (char == "\\") {
                        escaped = 1
                    } else if (char == "\"") {
                        end_quote_pos = i
                        break
                    }
                }
                
                if (end_quote_pos > 0) {
                    code_range_json = substr(rest_line, 2, end_quote_pos - 2)
                    
                    # 使用逗号切片，然后匹配每个字段
                    # 将 JSON 字符串按逗号分割
                    split(code_range_json, parts, ",")
                    
                    # 遍历每个部分，匹配需要的字段
                    for (i = 1; i <= length(parts); i++) {
                        part = parts[i]
                        # 匹配 start_line
                        if (match(part, /\\"start_line\\":([0-9]+)/, arr)) {
                            risk_data["start_line"] = arr[1]
                        }
                        # 匹配 end_line
                        else if (match(part, /\\"end_line\\":([0-9]+)/, arr)) {
                            risk_data["end_line"] = arr[1]
                        }
                        # 匹配 start_column
                        else if (match(part, /\\"start_column\\":([0-9]+)/, arr)) {
                            risk_data["start_column"] = arr[1]
                        }
                        # 匹配 end_column
                        else if (match(part, /\\"end_column\\":([0-9]+)/, arr)) {
                            risk_data["end_column"] = arr[1]
                        }
                    }
                }
            }
        }
    }
    next
}

# 检测到风险项结束
in_risk && /^\s*\}\s*[,}]?\s*$/ {
    in_risk = 0
    output_risk()
    next
}

# 输出风险信息
function output_risk() {
    if (current_risk != "" && risk_data["file"] != "" && risk_data["line"] > 0) {
        risk_count++
        
        # 使用title_verbose作为显示标题，如果没有则使用title
        display_title = risk_data["title_verbose"]
        if (display_title == "") {
            display_title = risk_data["title"]
        }
        
        # 从第二个 / 开始提取文件路径
        file_short = risk_data["file"]
        first_slash = index(file_short, "/")
        if (first_slash > 0) {
            second_slash = index(substr(file_short, first_slash + 1), "/")
            if (second_slash > 0) {
                file_short = substr(file_short, first_slash + second_slash + 1)
            }
        }
        
        # 输出详细信息到文件
        detail_file = results_dir "/risk_details_" risk_count ".txt"
        print "=== 风险 " risk_count " ===" > detail_file
        print "风险ID: " current_risk > detail_file
        print "文件路径: " file_short > detail_file
        print "行号: " risk_data["line"] > detail_file
        
        if (risk_data["start_line"] > 0 && risk_data["end_line"] > 0) {
            if (risk_data["start_line"] == risk_data["end_line"]) {
                print "代码范围: " risk_data["start_line"] "行" > detail_file
            } else {
                print "代码范围: " risk_data["start_line"] "-" risk_data["end_line"] "行" > detail_file
            }
            if (risk_data["start_column"] > 0 && risk_data["end_column"] > 0) {
                print "列范围: " risk_data["start_column"] "-" risk_data["end_column"] > detail_file
            }
        }
        
        print "严重程度: " risk_data["severity"] > detail_file
        print "标题: " risk_data["title"] > detail_file
        if (risk_data["title_verbose"] != "") {
            print "中文标题: " risk_data["title_verbose"] > detail_file
        }
        if (risk_data["rule_name"] != "") {
            print "规则名称: " risk_data["rule_name"] > detail_file
        }
        if (risk_data["function_name"] != "") {
            print "函数名称: " risk_data["function_name"] > detail_file
        }
        if (risk_data["program_name"] != "") {
            print "程序名称: " risk_data["program_name"] > detail_file
        }
        if (risk_data["language"] != "") {
            print "编程语言: " risk_data["language"] > detail_file
        }
        if (risk_data["risk_type"] != "") {
            print "风险类型: " risk_data["risk_type"] > detail_file
        }
        if (risk_data["cve"] != "") {
            print "CVE: " risk_data["cve"] > detail_file
        }
        if (risk_data["cwe"] != "") {
            print "CWE: " risk_data["cwe"] > detail_file
        }
        if (risk_data["time"] != "") {
            print "时间: " risk_data["time"] > detail_file
        }
        if (risk_data["latest_disposal_status"] != "") {
            print "处置状态: " risk_data["latest_disposal_status"] > detail_file
        }
        if (risk_data["code_fragment"] != "") {
            print "代码片段:" > detail_file
            print risk_data["code_fragment"] > detail_file
        }
        print "描述: " risk_data["description"] > detail_file
        print "解决方案: " risk_data["solution"] > detail_file
        print "" > detail_file
        
        # 在每个详细文件末尾添加统计信息
        print "=== 扫描统计 ===" > detail_file
        print "总风险数: " risk_count > detail_file
        print "当前风险: " risk_count " / " risk_count > detail_file
        print "扫描完成时间: " strftime("%Y-%m-%d %H:%M:%S") > detail_file
    }
}

END {
    # 生成总结文件到 results 目录
    summary_file = results_dir "/scan_summary.txt"
    print "=== 安全扫描总结报告 ===" > summary_file
    print "扫描时间: " strftime("%Y-%m-%d %H:%M:%S") > summary_file
    print "总风险数: " risk_count > summary_file
    print "" > summary_file
    
    if (risk_count > 0) {
        print "发现的安全风险:" > summary_file
        for (i = 1; i <= risk_count; i++) {
            detail_file = results_dir "/risk_details_" i ".txt"
            if (system("test -f " detail_file) == 0) {
                print "  风险 " i ": 详细信息请查看 risk_details_" i ".txt" > summary_file
            }
        }
        print "" > summary_file
        print "建议:" > summary_file
        print "1. 查看各个风险详情文件了解具体问题" > summary_file
        print "2. 根据解决方案建议进行代码修复" > summary_file
        print "3. 重新扫描验证修复效果" > summary_file
    } else {
        print "✅ 未发现安全风险，代码安全检查通过！" > summary_file
    }
    
    print "" > summary_file
    print "报告生成完成，所有文件保存在 " results_dir " 目录中" > summary_file
}
