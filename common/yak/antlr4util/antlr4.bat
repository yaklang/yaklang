@echo off
REM Unified ANTLR 4 Go codegen entrypoint for yaklang (Windows).
setlocal
set SCRIPT_DIR=%~dp0
set THIRDPARTY_DIR=%SCRIPT_DIR%..\antlr4thirdparty
set TEMPLATES_DIR=%THIRDPARTY_DIR%\templates
set JAR=%THIRDPARTY_DIR%\antlr-4.13.2-complete.jar

if not exist "%JAR%" (
  echo antlr4: missing jar: %JAR% 1>&2
  exit /b 1
)

java -cp "%TEMPLATES_DIR%;%JAR%" org.antlr.v4.Tool %*
exit /b %ERRORLEVEL%
