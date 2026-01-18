@echo off
setlocal enabledelayedexpansion

rem 显式定位到脚本所在目录的父目录（主程序目录）
cd /d "%~dp0"
cd ..

echo 开始更新流程...

set "APP_NAME=QMediaSync.exe"
set "BACKUP_DIR=old"
set "UPDATE_DIR=update"

:check_update_dir
rem 检查更新目录是否存在
if not exist "%UPDATE_DIR%" (
    echo 错误: 更新目录 %UPDATE_DIR% 不存在，停止更新。
    pause
    exit /b 1
)

:check_running
echo 检查程序是否运行...
tasklist /FI "IMAGENAME eq %APP_NAME%" 2>NUL | find /I "%APP_NAME%" >NUL
if "%ERRORLEVEL%"=="0" (
    echo 正在停止 %APP_NAME%...
    taskkill /IM "%APP_NAME%" /F
    timeout /t 3 /nobreak >NUL
    goto check_running
)

echo 程序已停止，开始更新...

:prepare_backup
echo 准备备份目录...
rem 如果备份目录存在，先删除
if exist "%BACKUP_DIR%" (
    echo 删除旧的备份目录...
    rd /s /q "%BACKUP_DIR%"
    if %ERRORLEVEL% neq 0 (
        echo 错误: 删除备份目录失败。
        pause
        exit /b 1
    )
)

rem 创建新的备份目录
mkdir "%BACKUP_DIR%"
if %ERRORLEVEL% neq 0 (
    echo 错误: 创建备份目录失败。
    pause
    exit /b 1
)

:backup_files
echo 备份文件...

rem 备份主程序
if exist "%APP_NAME%" (
    echo 备份 %APP_NAME%...
    copy "%APP_NAME%" "%BACKUP_DIR%\%APP_NAME%" >NUL
    if %ERRORLEVEL% neq 0 (
        echo 警告: 备份 %APP_NAME% 失败。
    )
)

rem 备份 web_statics 目录
if exist "web_statics" (
    echo 备份 web_statics 目录...
    mkdir "%BACKUP_DIR%\web_statics"
    xcopy /s /e /i "web_statics" "%BACKUP_DIR%\web_statics" >NUL
    if %ERRORLEVEL% neq 0 (
        echo 警告: 备份 web_statics 目录失败。
    )
)

rem 备份 scripts 目录
if exist "scripts" (
    echo 备份 scripts 目录...
    mkdir "%BACKUP_DIR%\scripts"
    xcopy /s /e /i "scripts" "%BACKUP_DIR%\scripts" >NUL
    if %ERRORLEVEL% neq 0 (
        echo 警告: 备份 scripts 目录失败。
    )
)

:update_files
echo 开始更新文件...

rem 更新主程序
if exist "%UPDATE_DIR%\%APP_NAME%" (
    echo 更新 %APP_NAME%...
    copy /y "%UPDATE_DIR%\%APP_NAME%" ".\" >NUL
    if %ERRORLEVEL% neq 0 (
        echo 错误: 更新 %APP_NAME% 失败。
        pause
        exit /b 1
    )
) else (
    echo 警告: 更新目录中未找到 %APP_NAME%。
)

rem 更新 web_statics 目录
if exist "%UPDATE_DIR%\web_statics" (
    echo 更新 web_statics 目录...
    rem 先删除现有目录
    if exist "web_statics" (
        rd /s /q "web_statics"
    )
    xcopy /s /e /i "%UPDATE_DIR%\web_statics" "web_statics" >NUL
    if %ERRORLEVEL% neq 0 (
        echo 错误: 更新 web_statics 目录失败。
        pause
        exit /b 1
    )
) else (
    echo 警告: 更新目录中未找到 web_statics 目录。
)

rem 更新 scripts 目录
if exist "%UPDATE_DIR%\scripts" (
    echo 更新 scripts 目录...
    rem 先删除现有目录
    if exist "scripts" (
        rd /s /q "scripts"
    )
    xcopy /s /e /i "%UPDATE_DIR%\scripts" "scripts" >NUL
    if %ERRORLEVEL% neq 0 (
        echo 错误: 更新 scripts 目录失败。
        pause
        exit /b 1
    )
) else (
    echo 警告: 更新目录中未找到 scripts 目录。
)

echo 更新完成!
echo 启动新版本...
start "" "%APP_NAME%"

endlocal