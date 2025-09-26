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
    
    # 输出格式
    printf "%-20s %-8s %-10s %-50s\n", "文件路径", "行号", "严重程度", "问题标题"
    printf "%-20s %-8s %-10s %-50s\n", "--------------------", "--------", "----------", "--------------------------------------------------"
}

# 检测到Risks字段开始 (注意是大写R)
/\"Risks\"\s*:\s*\{/ {
    in_risks = 1
    next
}

# 检测到Risks字段结束
in_risks && /^\s*\}\s*,?\s*$/ {
    in_risks = 0
    next
}

# 在Risks字段内，检测到新的风险项
in_risks && /\"[a-f0-9]{40}\"\s*:\s*\{/ {
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
    
    # 提取风险ID (40位哈希值)
    match($0, /\"([a-f0-9]{40})\"\s*:\s*\{/, arr)
    current_risk = arr[1]
    next
}

# 在风险项内，检测到code_source_url字段 (对应文件路径)
in_risk && /\"code_source_url\"\s*:\s*\"[^\"]*\"/ {
    match($0, /\"code_source_url\"\s*:\s*\"([^\"]*)\"/, arr)
    current_file = arr[1]
    next
}

# 检测line字段
in_risk && /\"line\"\s*:\s*[0-9]+/ {
    match($0, /\"line\"\s*:\s*([0-9]+)/, arr)
    current_line = arr[1]
    next
}

# 检测severity字段
in_risk && /\"severity\"\s*:\s*\"[^\"]*\"/ {
    match($0, /\"severity\"\s*:\s*\"([^\"]*)\"/, arr)
    current_severity = arr[1]
    next
}

# 检测title字段
in_risk && /\"title\"\s*:\s*\"[^\"]*\"/ {
    match($0, /\"title\"\s*:\s*\"([^\"]*)\"/, arr)
    current_title = arr[1]
    next
}

# 检测title_verbose字段
in_risk && /\"title_verbose\"\s*:\s*\"[^\"]*\"/ {
    match($0, /\"title_verbose\"\s*:\s*\"([^\"]*)\"/, arr)
    current_title_verbose = arr[1]
    next
}

# 检测description字段
in_risk && /\"description\"\s*:\s*\"[^\"]*\"/ {
    match($0, /\"description\"\s*:\s*\"([^\"]*)\"/, arr)
    current_description = arr[1]
    next
}

# 检测solution字段
in_risk && /\"solution\"\s*:\s*\"[^\"]*\"/ {
    match($0, /\"solution\"\s*:\s*\"([^\"]*)\"/, arr)
    current_solution = arr[1]
    next
}

# 检测rule_name字段
in_risk && /\"rule_name\"\s*:\s*\"[^\"]*\"/ {
    match($0, /\"rule_name\"\s*:\s*\"([^\"]*)\"/, arr)
    current_rule_name = arr[1]
    next
}

# 检测function_name字段
in_risk && /\"function_name\"\s*:\s*\"[^\"]*\"/ {
    match($0, /\"function_name\"\s*:\s*\"([^\"]*)\"/, arr)
    current_function_name = arr[1]
    next
}

# 检测program_name字段
in_risk && /\"program_name\"\s*:\s*\"[^\"]*\"/ {
    match($0, /\"program_name\"\s*:\s*\"([^\"]*)\"/, arr)
    current_program_name = arr[1]
    next
}

# 检测language字段
in_risk && /\"language\"\s*:\s*\"[^\"]*\"/ {
    match($0, /\"language\"\s*:\s*\"([^\"]*)\"/, arr)
    current_language = arr[1]
    next
}

# 检测risk_type字段
in_risk && /\"risk_type\"\s*:\s*\"[^\"]*\"/ {
    match($0, /\"risk_type\"\s*:\s*\"([^\"]*)\"/, arr)
    current_risk_type = arr[1]
    next
}

# 检测cve字段
in_risk && /\"cve\"\s*:\s*\"[^\"]*\"/ {
    match($0, /\"cve\"\s*:\s*\"([^\"]*)\"/, arr)
    current_cve = arr[1]
    next
}

# 检测cwe字段 (数组格式)
in_risk && /\"cwe\"\s*:\s*\[/ {
    # 简单处理，提取数组内容
    match($0, /\"cwe\"\s*:\s*\[([^\]]*)\]/, arr)
    current_cwe = arr[1]
    next
}

# 检测time字段
in_risk && /\"time\"\s*:\s*\"[^\"]*\"/ {
    match($0, /\"time\"\s*:\s*\"([^\"]*)\"/, arr)
    current_time = arr[1]
    next
}

# 检测latest_disposal_status字段
in_risk && /\"latest_disposal_status\"\s*:\s*\"[^\"]*\"/ {
    match($0, /\"latest_disposal_status\"\s*:\s*\"([^\"]*)\"/, arr)
    current_latest_disposal_status = arr[1]
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
        
        # 截断过长的标题
        title_short = display_title
        if (length(title_short) > 50) {
            title_short = substr(title_short, 1, 47) "..."
        }
        
        # 截断过长的文件路径
        file_short = current_file
        if (length(file_short) > 20) {
            file_short = "..." substr(file_short, length(file_short) - 16)
        }
        
        printf "%-20s %-8s %-10s %-50s\n", file_short, current_line, current_severity, title_short
        
        # 输出详细信息到文件
        detail_file = "risk_details_" risk_count ".txt"
        print "=== 风险 " risk_count " ===" > detail_file
        print "风险ID: " current_risk > detail_file
        print "文件路径: " current_file > detail_file
        print "行号: " current_line > detail_file
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
        print "描述: " current_description > detail_file
        print "解决方案: " current_solution > detail_file
        print "" > detail_file
    }
}

END {
    printf "\n总共找到 %d 个风险\n", risk_count
    if (risk_count > 0) {
        print "详细信息已保存到 risk_details_*.txt 文件中"
    }
}
