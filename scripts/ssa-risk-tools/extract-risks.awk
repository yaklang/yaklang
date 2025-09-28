#!/usr/bin/awk -f

# extract-risks.awk
# 从JSON格式的风险报告中提取问题代码信息
# 用法: awk -f extract-risks.awk risk.json

BEGIN {
    # 设置字段分隔符
    FS = ""
    
    # 初始化变量
    in_risks = 0
    in_risk = 0
    risk_count = 0
    current_risk = ""
    current_file = ""
    current_line = 0
    current_severity = ""
    current_title = ""
    current_title_verbose = ""
    current_description = ""
    current_solution = ""
    current_rule_name = ""
    current_function_name = ""
    current_program_name = ""
    current_language = ""
    current_risk_type = ""
    current_cve = ""
    current_cwe = ""
    current_time = ""
    current_latest_disposal_status = ""
    current_code_range = ""
    current_start_line = 0
    current_end_line = 0
    current_start_column = 0
    current_end_column = 0
    current_code_fragment = ""
    
    # 创建 results 目录
    results_dir = "results"
    system("mkdir -p " results_dir)

    # 移除屏幕输出格式（只保留文件输出）
}

# 检测到Risks字段开始 (注意是大写R)
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
    current_file = ""
    current_line = 0
    current_severity = ""
    current_title = ""
    current_title_verbose = ""
    current_description = ""
    current_solution = ""
    current_rule_name = ""
    current_function_name = ""
    current_program_name = ""
    current_language = ""
    current_risk_type = ""
    current_cve = ""
    current_cwe = ""
    current_time = ""
    current_latest_disposal_status = ""
    current_code_range = ""
    current_start_line = 0
    current_end_line = 0
    current_start_column = 0
    current_end_column = 0
    current_code_fragment = ""
    
    # 提取风险ID (40位哈希值)
    match($0, /"([a-f0-9]{40})"[[:space:]]*:[[:space:]]*{/, arr)
    current_risk = arr[1]
    next
}

# 在风险项内，检测到code_source_url字段 (对应文件路径)
in_risk && /"code_source_url"[[:space:]]*:[[:space:]]*"[^"]*"/ {
    match($0, /"code_source_url"[[:space:]]*:[[:space:]]*"([^"]*)"/, arr)
    current_file = arr[1]
    next
}

# 检测line字段
in_risk && /"line"[[:space:]]*:[[:space:]]*[0-9]+/ {
    match($0, /"line"[[:space:]]*:[[:space:]]*([0-9]+)/, arr)
    current_line = arr[1]
    next
}

# 检测severity字段
in_risk && /"severity"[[:space:]]*:[[:space:]]*"[^"]*"/ {
    match($0, /"severity"[[:space:]]*:[[:space:]]*"([^"]*)"/, arr)
    current_severity = arr[1]
    next
}

# 检测title字段
in_risk && /"title"[[:space:]]*:[[:space:]]*"[^"]*"/ {
    match($0, /"title"[[:space:]]*:[[:space:]]*"([^"]*)"/, arr)
    current_title = arr[1]
    next
}

# 检测title_verbose字段
in_risk && /"title_verbose"[[:space:]]*:[[:space:]]*"[^"]*"/ {
    match($0, /"title_verbose"[[:space:]]*:[[:space:]]*"([^"]*)"/, arr)
    current_title_verbose = arr[1]
    next
}

# 检测description字段
in_risk && /"description"[[:space:]]*:[[:space:]]*"[^"]*"/ {
    match($0, /"description"[[:space:]]*:[[:space:]]*"([^"]*)"/, arr)
    current_description = arr[1]
    next
}

# 检测solution字段
in_risk && /"solution"[[:space:]]*:[[:space:]]*"[^"]*"/ {
    match($0, /"solution"[[:space:]]*:[[:space:]]*"([^"]*)"/, arr)
    current_solution = arr[1]
    next
}

# 检测rule_name字段
in_risk && /"rule_name"[[:space:]]*:[[:space:]]*"[^"]*"/ {
    match($0, /"rule_name"[[:space:]]*:[[:space:]]*"([^"]*)"/, arr)
    current_rule_name = arr[1]
    next
}

# 检测function_name字段
in_risk && /"function_name"[[:space:]]*:[[:space:]]*"[^"]*"/ {
    match($0, /"function_name"[[:space:]]*:[[:space:]]*"([^"]*)"/, arr)
    current_function_name = arr[1]
    next
}

# 检测program_name字段
in_risk && /"program_name"[[:space:]]*:[[:space:]]*"[^"]*"/ {
    match($0, /"program_name"[[:space:]]*:[[:space:]]*"([^"]*)"/, arr)
    current_program_name = arr[1]
    next
}

# 检测language字段
in_risk && /"language"[[:space:]]*:[[:space:]]*"[^"]*"/ {
    match($0, /"language"[[:space:]]*:[[:space:]]*"([^"]*)"/, arr)
    current_language = arr[1]
    next
}

# 检测risk_type字段
in_risk && /"risk_type"[[:space:]]*:[[:space:]]*"[^"]*"/ {
    match($0, /"risk_type"[[:space:]]*:[[:space:]]*"([^"]*)"/, arr)
    current_risk_type = arr[1]
    next
}

# 检测cve字段
in_risk && /"cve"[[:space:]]*:[[:space:]]*"[^"]*"/ {
    match($0, /"cve"[[:space:]]*:[[:space:]]*"([^"]*)"/, arr)
    current_cve = arr[1]
    next
}

# 检测cwe字段 (数组格式)
in_risk && /"cwe"[[:space:]]*:[[:space:]]*\[/ {
    # 简单处理，提取数组内容
    match($0, /"cwe"[[:space:]]*:[[:space:]]*\[([^\]]*)\]/, arr)
    current_cwe = arr[1]
    next
}

# 检测time字段
in_risk && /"time"[[:space:]]*:[[:space:]]*"[^"]*"/ {
    match($0, /"time"[[:space:]]*:[[:space:]]*"([^"]*)"/, arr)
    current_time = arr[1]
    next
}

# 检测latest_disposal_status字段
in_risk && /"latest_disposal_status"[[:space:]]*:[[:space:]]*"[^"]*"/ {
    match($0, /"latest_disposal_status"[[:space:]]*:[[:space:]]*"([^"]*)"/, arr)
    current_latest_disposal_status = arr[1]
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
                            current_start_line = arr[1]
                        }
                        # 匹配 end_line
                        else if (match(part, /\\"end_line\\":([0-9]+)/, arr)) {
                            current_end_line = arr[1]
                        }
                        # 匹配 start_column
                        else if (match(part, /\\"start_column\\":([0-9]+)/, arr)) {
                            current_start_column = arr[1]
                        }
                        # 匹配 end_column
                        else if (match(part, /\\"end_column\\":([0-9]+)/, arr)) {
                            current_end_column = arr[1]
                        }
                    }
                }
            }
        }
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
                    # 提取 code_fragment 内容，处理转义字符
                    code_fragment_raw = substr(rest_line, 2, end_quote_pos - 2)
                    
                    # 处理常见的转义字符
                    current_code_fragment = code_fragment_raw
                    # 替换 \n 为真正的换行符
                    gsub(/\\n/, "\n", current_code_fragment)
                    # 替换 \t 为真正的制表符
                    gsub(/\\t/, "\t", current_code_fragment)
                    # 替换 \" 为 "
                    gsub(/\\"/, "\"", current_code_fragment)
                    # 替换 \\ 为 \
                    gsub(/\\\\/, "\\", current_code_fragment)
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
    if (current_risk != "" && current_file != "" && current_line > 0) {
        risk_count++
        
        # 使用title_verbose作为显示标题，如果没有则使用title
        display_title = current_title_verbose
        if (display_title == "") {
            display_title = current_title
        }
        
        # 不截断标题，保持完整信息
        title_short = display_title
        
        # 从第二个 / 开始提取文件路径（不截断）
        file_short = current_file
        # 找到第二个 / 的位置
        first_slash = index(file_short, "/")
        if (first_slash > 0) {
            second_slash = index(substr(file_short, first_slash + 1), "/")
            if (second_slash > 0) {
                # 从第二个 / 开始提取（不包含第二个 /）
                file_short = substr(file_short, first_slash + second_slash + 1)
            }
        }
        
        # 移除屏幕输出（只保留文件输出）
        
        # 输出详细信息到文件
        detail_file = results_dir "/risk_details_" risk_count ".txt"
        print "=== 风险 " risk_count " ===" > detail_file
        print "风险ID: " current_risk > detail_file
        print "文件路径: " file_short > detail_file
        print "行号: " current_line > detail_file
        if (current_start_line > 0 && current_end_line > 0) {
            if (current_start_line == current_end_line) {
                print "代码范围: " current_start_line "行" > detail_file
            } else {
                print "代码范围: " current_start_line "-" current_end_line "行" > detail_file
            }
            if (current_start_column > 0 && current_end_column > 0) {
                print "列范围: " current_start_column "-" current_end_column > detail_file
            }
        }
        print "严重程度: " current_severity > detail_file
        print "标题: " current_title > detail_file
        if (current_title_verbose != "") {
            print "中文标题: " current_title_verbose > detail_file
        }
        if (current_rule_name != "") {
            print "规则名称: " current_rule_name > detail_file
        }
        if (current_function_name != "") {
            print "函数名称: " current_function_name > detail_file
        }
        if (current_program_name != "") {
            print "程序名称: " current_program_name > detail_file
        }
        if (current_language != "") {
            print "编程语言: " current_language > detail_file
        }
        if (current_risk_type != "") {
            print "风险类型: " current_risk_type > detail_file
        }
        if (current_cve != "") {
            print "CVE: " current_cve > detail_file
        }
        if (current_cwe != "") {
            print "CWE: " current_cwe > detail_file
        }
        if (current_time != "") {
            print "时间: " current_time > detail_file
        }
        if (current_latest_disposal_status != "") {
            print "处置状态: " current_latest_disposal_status > detail_file
        }
        if (current_code_fragment != "") {
            print "代码片段:" > detail_file
            print current_code_fragment > detail_file
        }
        print "描述: " current_description > detail_file
        print "解决方案: " current_solution > detail_file
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
