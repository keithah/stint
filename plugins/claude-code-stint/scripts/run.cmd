@echo off
set ROOT=%CLAUDE_PLUGIN_ROOT%
if "%ROOT%"=="" set ROOT=%~dp0..
set STINT_PLUGIN_NAME=claude-code-stint
set STINT_PLUGIN_AGENT=claude
set STINT_PLUGIN_HOST=claude-code
set STINT_PLUGIN_HOST_VERSION_ENV=CLAUDE_CODE_VERSION
node "%ROOT%\..\shared\stint-plugin-runner.js" --background %*
