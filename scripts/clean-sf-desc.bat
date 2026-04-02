@echo off
setlocal EnableExtensions
rem SyntaxFlow .sf cleaner (entry: this BAT file).
rem   - literal "xxxxx"  -> ""
rem   - <<<DESC ... DESC -> <<<DESC + empty line + DESC
rem Dependency: clean-sf-desc.ps1 (same directory).
rem
rem Usage:
rem   clean-sf-desc.bat
rem   clean-sf-desc.bat path\to\folder
rem   clean-sf-desc.bat path\to\file.sf

set "SF_CLEAN_PATH=%~1"
if "%SF_CLEAN_PATH%"=="" set "SF_CLEAN_PATH=%~dp0.."

powershell -NoProfile -ExecutionPolicy Bypass -File "%~dp0clean-sf-desc.ps1" "%SF_CLEAN_PATH%"
if errorlevel 1 exit /b 1
endlocal
exit /b 0

