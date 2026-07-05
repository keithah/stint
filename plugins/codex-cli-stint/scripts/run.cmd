@echo off
set ROOT=%PLUGIN_ROOT%
if "%ROOT%"=="" set ROOT=%~dp0..
set STINT_PLUGIN_NAME=codex-cli-stint
set STINT_PLUGIN_AGENT=codex
set STINT_PLUGIN_HOST=codex-cli
set STINT_PLUGIN_HOST_VERSION_ENV=CODEX_CLI_VERSION
node "%ROOT%\..\shared\stint-plugin-runner.js" --background %*
