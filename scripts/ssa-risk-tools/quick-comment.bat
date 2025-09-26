@echo off
REM GitHub Comment Tool 快速评论脚本
REM 用于在Windows环境下快速执行GitHub评论

setlocal enabledelayedexpansion

echo 🚀 GitHub Comment Tool - 快速评论
echo =====================================

REM 检查Python是否可用
python --version >nul 2>&1
if errorlevel 1 (
    echo ❌ 错误: 未找到Python，请确保Python已安装并在PATH中
    pause
    exit /b 1
)

REM 检查配置文件
if not exist ".github\github-commenter.yml" (
    echo ❌ 错误: 未找到配置文件 .github\github-commenter.yml
    echo 请确保配置文件存在
    pause
    exit /b 1
)

REM 检查风险报告文件
set "risk_file="
if exist "risk.json" (
    set "risk_file=risk.json"
) else if exist "scripts\ssa-risk-tools\risk.json" (
    set "risk_file=scripts\ssa-risk-tools\risk.json"
) else (
    echo ❌ 错误: 未找到风险报告文件
    echo 请确保以下文件之一存在:
    echo   - risk.json
    echo   - scripts\ssa-risk-tools\risk.json
    pause
    exit /b 1
)

echo ✅ 找到风险报告文件: !risk_file!

REM 检查GitHub Token
if "%GITHUB_TOKEN%"=="" (
    echo ⚠️  警告: 未设置 GITHUB_TOKEN 环境变量
    echo 请设置环境变量或使用 -t 参数提供Token
    echo.
    echo 设置方法:
    echo   set GITHUB_TOKEN=your_token_here
    echo.
    echo 或者使用 -t 参数:
    echo   quick-comment.bat -t your_token_here -p PR_NUMBER
    echo.
)

REM 解析命令行参数
set "token="
set "pr_number="
set "dry_run="

:parse_args
if "%~1"=="" goto :args_done
if "%~1"=="-t" (
    set "token=%~2"
    shift
    shift
    goto :parse_args
)
if "%~1"=="-p" (
    set "pr_number=%~2"
    shift
    shift
    goto :parse_args
)
if "%~1"=="--dry-run" (
    set "dry_run=--dry-run"
    shift
    goto :parse_args
)
if "%~1"=="-h" goto :show_help
if "%~1"=="--help" goto :show_help
shift
goto :parse_args

:args_done

REM 检查必要参数
if "%pr_number%"=="" goto :show_help

REM 构建命令
set "cmd=python scripts\ssa-risk-tools\github_commenter.py -p %pr_number%"
if not "%token%"=="" set "cmd=%cmd% -t %token%"
if not "%dry_run%"=="" set "cmd=%cmd% %dry_run%"

echo.
echo 🔧 执行命令: %cmd%
echo.

REM 执行命令
%cmd%

echo.
echo 🎉 操作完成
pause
exit /b 0

:show_help
echo.
echo 用法: quick-comment.bat [选项] -p PR_NUMBER
echo.
echo 选项:
echo   -t TOKEN      GitHub Personal Access Token
echo   -p PR_NUMBER  Pull Request编号 (必需)
echo   --dry-run     干运行模式，只显示将要创建的评论
echo   -h, --help    显示此帮助信息
echo.
echo 示例:
echo   quick-comment.bat -p 123
echo   quick-comment.bat -t ghp_xxx -p 123
echo   quick-comment.bat -p 123 --dry-run
echo.
echo 环境变量:
echo   GITHUB_TOKEN  GitHub Personal Access Token
echo.
pause
exit /b 0
