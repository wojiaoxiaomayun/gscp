@echo off
chcp 65001 >nul 2>&1
setlocal enabledelayedexpansion

echo ============================================
echo        GSCP Windows 一键安装脚本
echo ============================================
echo.

:: 设置安装目录
set "INSTALL_DIR=%USERPROFILE%\gscp"

:: 检查是否已安装
if exist "%INSTALL_DIR%\gscp.exe" (
    echo [信息] 检测到已安装 GSCP，将更新到最新版本...
    echo.
)

:: 创建安装目录
echo [1/3] 创建安装目录: %INSTALL_DIR%
if not exist "%INSTALL_DIR%" (
    mkdir "%INSTALL_DIR%"
    if errorlevel 1 (
        echo [错误] 无法创建目录 %INSTALL_DIR%
        pause
        exit /b 1
    )
    echo        目录创建成功
) else (
    echo        目录已存在
)
echo.

:: 下载 GSCP
echo [2/3] 下载 gscp.exe...
echo        下载地址: https://github.com/wojiaoxiaomayun/gscp/releases/download/v1.0.3/gscp.exe
echo.

:: 尝试使用 curl 下载
curl -L -o "%INSTALL_DIR%\gscp.exe" "https://github.com/wojiaoxiaomayun/gscp/releases/download/v1.0.3/gscp.exe" 2>nul
if errorlevel 1 (
    :: 如果 curl 不可用，尝试使用 PowerShell
    echo        curl 不可用，尝试使用 PowerShell 下载...
    powershell -Command "& {$ProgressPreference='SilentlyContinue'; Invoke-WebRequest -Uri 'https://github.com/wojiaoxiaomayun/gscp/releases/download/v1.0.3/gscp.exe' -OutFile '%INSTALL_DIR%\gscp.exe'}"
    if errorlevel 1 (
        echo [错误] 下载失败，请检查网络连接或手动下载
        pause
        exit /b 1
    )
)

:: 验证下载
if not exist "%INSTALL_DIR%\gscp.exe" (
    echo [错误] 下载失败，文件不存在
    pause
    exit /b 1
)

:: 检查文件大小（至少 1MB）
for %%A in ("%INSTALL_DIR%\gscp.exe") do set "FILESIZE=%%~zA"
if !FILESIZE! LSS 1048576 (
    echo [错误] 下载的文件大小异常，请重试或手动下载
    del "%INSTALL_DIR%\gscp.exe" 2>nul
    pause
    exit /b 1
)

echo        下载成功!
echo.

:: 设置环境变量
echo [3/3] 配置环境变量...

:: 检查 PATH 中是否已包含安装目录
echo !PATH! | findstr /I /C:"%INSTALL_DIR%" >nul
if not errorlevel 1 (
    echo        环境变量已配置
    goto :DONE
)

:: 使用 PowerShell 追加到用户 PATH（避免 setx 截断问题）
powershell -Command "$userPath = [Environment]::GetEnvironmentVariable('Path', 'User'); if ($userPath -notlike '*%INSTALL_DIR%*') { [Environment]::SetEnvironmentVariable('Path', \"$userPath;%INSTALL_DIR%\", 'User'); Write-Host '        环境变量设置成功' } else { Write-Host '        环境变量已配置' }"

if errorlevel 1 (
    echo [警告] 自动设置环境变量失败，请手动添加以下路径到 PATH:
    echo        %INSTALL_DIR%
)
echo.

:DONE
echo ============================================
echo           安装完成！
echo ============================================
echo.
echo 安装路径: %INSTALL_DIR%\gscp.exe
echo.
echo [重要] 请重新打开命令行窗口以使环境变量生效
echo.
echo 验证安装: 打开新的命令行窗口，运行:
echo           gscp --version
echo.
echo 使用帮助: gscp --help
echo ============================================
echo.

pause
