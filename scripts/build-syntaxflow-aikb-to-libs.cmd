@echo off
REM 构建 syntaxflow-aikb.rag 与 syntaxflow-aikb.zip 到 thirdparty 安装路径
REM 用法：在 yaklang 仓库根目录执行 scripts\build-syntaxflow-aikb-to-libs.cmd
REM 输出路径：%YAKIT_HOME%\projects\libs\ （未设置则用 %USERPROFILE%\yakit-projects）

cd /d "%~dp0\.."

if "%YAKIT_HOME%"=="" set "YAKIT_HOME=%USERPROFILE%\yakit-projects"
set "LIBS_DIR=%YAKIT_HOME%\projects\libs"
if not exist "%LIBS_DIR%" mkdir "%LIBS_DIR%"

if not exist "syntaxflow-aikb" (
  echo Error: syntaxflow-aikb not found, run from yaklang repo root
  exit /b 1
)

where yak >nul 2>nul
if errorlevel 1 (
  echo Error: yak not in PATH
  exit /b 1
)

echo [build] Install dir: %LIBS_DIR%
echo [build] Building syntaxflow-aikb.zip...
yak syntaxflow-aikb/scripts/merge-in-one-text.yak --base syntaxflow-aikb --output "%LIBS_DIR%\syntaxflow-aikb.zip"
if errorlevel 1 exit /b 1

echo [build] Building syntaxflow-aikb.rag...
yak syntaxflow-aikb/scripts/build-syntaxflow-aikb-rag.yak --base syntaxflow-aikb --output "%LIBS_DIR%\syntaxflow-aikb.rag"
if errorlevel 1 exit /b 1

echo [build] Done:
echo   - %LIBS_DIR%\syntaxflow-aikb.zip
echo   - %LIBS_DIR%\syntaxflow-aikb.rag
