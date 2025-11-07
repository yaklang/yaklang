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
    file_is_empty = 1  # 标记文件是否为空
    
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
    if (system("[ -d '" results_dir "' ]") != 0) {
        if (system("mkdir -p '" results_dir "'") != 0) {
            system("powershell -NoProfile -Command \"New-Item -ItemType Directory -Force -Path '" results_dir "' | Out-Null\"")
        }
    }
}

# 通用JSON字符串解析函数
function parse_json_string(json_str, result) {
    # 移除首尾引号
    if (substr(json_str, 1, 1) == "\"" && substr(json_str, length(json_str), 1) == "\"") {
        json_str = substr(json_str, 2, length(json_str) - 2)
    }
    
    return json_str
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

# 统一的简单字段提取函数（单行 JSON 字符串）
function extract_simple_field(line, field_name) {
    pattern = "\"" field_name "\"[[:space:]]*:[[:space:]]*\"([^\"]*)\""
    if (match(line, pattern, arr)) {
        return arr[1]
    }
    return ""
}

# 统一的数字字段提取函数
function extract_number_field(line, field_name) {
    pattern = "\"" field_name "\"[[:space:]]*:[[:space:]]*([0-9]+)"
    if (match(line, pattern, arr)) {
        return arr[1]
    }
    return 0
}

# 查找字符串中的结束引号位置（跳过转义的引号）
function find_end_quote(str, start_pos) {
    escaped = 0
    for (i = start_pos; i <= length(str); i++) {
        char = substr(str, i, 1)
        if (escaped) {
            escaped = 0
        } else if (char == "\\") {
            escaped = 1
        } else if (char == "\"") {
            return i
        }
    }
    return 0
}

# 统一的转义字符处理函数
function unescape_json_string(str) {
    # 处理 JSON 转义字符，按照正确的顺序
    # 先处理 \r\n (Windows 换行)
    gsub(/\\r\\n/, "\n", str)
    # 再处理单独的 \n
    gsub(/\\n/, "\n", str)
    # 处理 \r
    gsub(/\\r/, "\r", str)
    # 处理制表符
    gsub(/\\t/, "\t", str)
    # 处理引号
    gsub(/\\"/, "\"", str)
    # 处理 Unicode 转义（常见的 HTML 实体）
    gsub(/\\u003c/, "<", str)
    gsub(/\\u003e/, ">", str)
    gsub(/\\u0026/, "\\&", str)
    # 处理反斜杠（必须最后处理）
    gsub(/\\\\/, "\\", str)
    return str
}

# 处理任何非空行，标记文件不为空
NF > 0 {
    file_is_empty = 0
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
/"[a-f0-9]{40}"\s*:\s*\{/ {
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

# 统一的简单字段检测和提取
in_risk && /"(code_source_url|severity|title|title_verbose|rule_name|function_name|program_name|language|risk_type|cve|time|latest_disposal_status)"[[:space:]]*:[[:space:]]*"[^"]*"/ {
    # 检测并提取各种简单字段
    if (value = extract_simple_field($0, "code_source_url")) risk_data["file"] = value
    else if (value = extract_simple_field($0, "severity")) {
        gsub(/^[[:space:]]+|[[:space:]]+$/, "", value)
        value = tolower(value)
        risk_data["severity"] = value
    }
    else if (value = extract_simple_field($0, "title")) risk_data["title"] = value
    else if (value = extract_simple_field($0, "title_verbose")) risk_data["title_verbose"] = value
    else if (value = extract_simple_field($0, "rule_name")) risk_data["rule_name"] = value
    else if (value = extract_simple_field($0, "function_name")) risk_data["function_name"] = value
    else if (value = extract_simple_field($0, "program_name")) risk_data["program_name"] = value
    else if (value = extract_simple_field($0, "language")) risk_data["language"] = value
    else if (value = extract_simple_field($0, "risk_type")) risk_data["risk_type"] = value
    else if (value = extract_simple_field($0, "cve")) risk_data["cve"] = value
    else if (value = extract_simple_field($0, "time")) risk_data["time"] = value
    else if (value = extract_simple_field($0, "latest_disposal_status")) risk_data["latest_disposal_status"] = value
    next
}

# 检测line字段（数字类型）
in_risk && /"line"[[:space:]]*:[[:space:]]*[0-9]+/ {
    risk_data["line"] = extract_number_field($0, "line")
    next
}

# 检测cwe字段 (数组格式)
in_risk && /"cwe"[[:space:]]*:[[:space:]]*\[/ {
    if (match($0, /"cwe"[[:space:]]*:[[:space:]]*\[([^\]]*)\]/, arr)) {
    risk_data["cwe"] = arr[1]
    }
    next
}

# 统一处理多行字符串字段（description, solution, code_fragment）
function start_multiline_field(field_name, line) {
    start_pos = index(line, field_name)
    if (start_pos == 0) return 0
    
    colon_pos = index(substr(line, start_pos), ":")
    if (colon_pos == 0) return 0
    
    quote_start = index(substr(line, start_pos + colon_pos), "\"")
    if (quote_start == 0) return 0
    
    rest_line = substr(line, start_pos + colon_pos + quote_start)
    end_quote_pos = find_end_quote(rest_line, 2)
    
    if (end_quote_pos > 0) {
        # 单行完整字符串，需要转义处理
        raw_content = substr(rest_line, 2, end_quote_pos - 2)
        risk_data[field_name] = unescape_json_string(raw_content)
        return 1  # 完成
    } else {
        # 多行字符串，开始累积
        return 2  # 需要继续读取
    }
}

# 检测并处理 description 字段
in_risk && !in_description && !in_solution && !in_code_fragment && /"description"[[:space:]]*:[[:space:]]*"/ {
    result = start_multiline_field("description", $0)
    if (result == 2) {
        # 多行，开始累积
    start_pos = index($0, "description")
        colon_pos = index(substr($0, start_pos), ":")
            quote_start = index(substr($0, start_pos + colon_pos), "\"")
                rest_line = substr($0, start_pos + colon_pos + quote_start)
        description_raw = substr(rest_line, 2)
                    in_description = 1
    }
    next
}

# 处理多行 description
in_risk && in_description {
    end_quote_pos = find_end_quote($0, 1)
    if (end_quote_pos > 0) {
        description_raw = description_raw substr($0, 1, end_quote_pos - 1)
        risk_data["description"] = unescape_json_string(description_raw)
        in_description = 0
        description_raw = ""
    } else {
        description_raw = description_raw $0
    }
    next
}

# 检测并处理 solution 字段
in_risk && !in_description && !in_solution && !in_code_fragment && /"solution"[[:space:]]*:[[:space:]]*"/ {
    result = start_multiline_field("solution", $0)
    if (result == 2) {
    start_pos = index($0, "solution")
        colon_pos = index(substr($0, start_pos), ":")
            quote_start = index(substr($0, start_pos + colon_pos), "\"")
                rest_line = substr($0, start_pos + colon_pos + quote_start)
        solution_raw = substr(rest_line, 2)
                    in_solution = 1
    }
    next
}

# 处理多行 solution
in_risk && in_solution {
    end_quote_pos = find_end_quote($0, 1)
    if (end_quote_pos > 0) {
        solution_raw = solution_raw substr($0, 1, end_quote_pos - 1)
        risk_data["solution"] = unescape_json_string(solution_raw)
        in_solution = 0
        solution_raw = ""
    } else {
        solution_raw = solution_raw $0
    }
    next
}

# 检测并处理 code_fragment 字段
in_risk && !in_description && !in_solution && !in_code_fragment && /"code_fragment"[[:space:]]*:[[:space:]]*"/ {
    result = start_multiline_field("code_fragment", $0)
    if (result == 2) {
    start_pos = index($0, "code_fragment")
        colon_pos = index(substr($0, start_pos), ":")
            quote_start = index(substr($0, start_pos + colon_pos), "\"")
                rest_line = substr($0, start_pos + colon_pos + quote_start)
        code_fragment_raw = substr(rest_line, 2)
                    in_code_fragment = 1
    }
    next
}

# 处理多行 code_fragment
in_risk && in_code_fragment {
    end_quote_pos = find_end_quote($0, 1)
    if (end_quote_pos > 0) {
        code_fragment_raw = code_fragment_raw substr($0, 1, end_quote_pos - 1)
        risk_data["code_fragment"] = unescape_json_string(code_fragment_raw)
        in_code_fragment = 0
        code_fragment_raw = ""
    } else {
        code_fragment_raw = code_fragment_raw $0
    }
    next
}

# 检测code_range字段并提取位置信息
in_risk && /"code_range"[[:space:]]*:[[:space:]]*"/ {
    if (match($0, /"code_range"[[:space:]]*:[[:space:]]*"([^"]*)"/, arr)) {
        code_range_json = arr[1]
        # 提取各个位置字段
        if (match(code_range_json, /\\"start_line\\":([0-9]+)/, arr)) risk_data["start_line"] = arr[1]
        if (match(code_range_json, /\\"end_line\\":([0-9]+)/, arr)) risk_data["end_line"] = arr[1]
        if (match(code_range_json, /\\"start_column\\":([0-9]+)/, arr)) risk_data["start_column"] = arr[1]
        if (match(code_range_json, /\\"end_column\\":([0-9]+)/, arr)) risk_data["end_column"] = arr[1]
    }
    next
}

# 检测到风险项结束
in_risk && /^\s*\}\s*[,}]?\s*$/ {
    in_risk = 0
    output_risk()
    next
}

# 输出单个字段（如果非空）
function print_field_if_exists(field_label, field_value, file) {
    if (field_value != "") {
        print field_label ": " field_value > file
    }
}

# 输出风险信息
function output_risk() {
    if (current_risk == "" || risk_data["file"] == "" || risk_data["line"] <= 0) {
        return
    }
    
    # 过滤严重程度，只保留 critical 和 high
    if (risk_data["severity"] != "critical" && risk_data["severity"] != "high") {
        return
    }
    
        risk_count++
        
    # 提取短文件路径（从第二个斜杠开始）
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
        
    # 输出代码范围
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
        
    # 输出必需字段
        print "严重程度: " risk_data["severity"] > detail_file
        print "标题: " risk_data["title"] > detail_file
    
    # 输出可选字段
    print_field_if_exists("中文标题", risk_data["title_verbose"], detail_file)
    print_field_if_exists("规则名称", risk_data["rule_name"], detail_file)
    print_field_if_exists("函数名称", risk_data["function_name"], detail_file)
    print_field_if_exists("程序名称", risk_data["program_name"], detail_file)
    print_field_if_exists("编程语言", risk_data["language"], detail_file)
    print_field_if_exists("风险类型", risk_data["risk_type"], detail_file)
    print_field_if_exists("CVE", risk_data["cve"], detail_file)
    print_field_if_exists("CWE", risk_data["cwe"], detail_file)
    print_field_if_exists("时间", risk_data["time"], detail_file)
    print_field_if_exists("处置状态", risk_data["latest_disposal_status"], detail_file)
    
    # 输出代码片段
        if (risk_data["code_fragment"] != "") {
            print "代码片段:" > detail_file
        print "```go" > detail_file
            print risk_data["code_fragment"] > detail_file
        print "```" > detail_file
    }
    
    # 输出描述和解决方案
    print "描述:" > detail_file
    print risk_data["description"] > detail_file
    print "" > detail_file
    print "解决方案:" > detail_file
    print risk_data["solution"] > detail_file
        print "" > detail_file
        
    # 输出统计信息
        print "=== 扫描统计 ===" > detail_file
        print "总风险数: " risk_count > detail_file
        print "当前风险: " risk_count " / " risk_count > detail_file
        print "扫描完成时间: " strftime("%Y-%m-%d %H:%M:%S") > detail_file

    current_risk = ""
    for (key in risk_data) {
        risk_data[key] = ""
    }
}

END {
    # 输出最后一个风险（如果存在）
    if (current_risk != "") {
        output_risk()
    }
    
    summary_file = results_dir "/scan_summary.txt"
    print "=== 安全扫描总结报告 ===" > summary_file
    print "扫描时间: " strftime("%Y-%m-%d %H:%M:%S") > summary_file
    print "总风险数: " risk_count > summary_file
    print "" > summary_file
    
    if (file_is_empty) {
        print "⚠️ 输入文件为空，未进行安全扫描" > summary_file
        print "这通常意味着没有发现安全风险" > summary_file
    } else if (risk_count > 0) {
        print "发现的安全风险:" > summary_file
        for (i = 1; i <= risk_count; i++) {
                print "  风险 " i ": 详细信息请查看 risk_details_" i ".txt" > summary_file
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
