#!/bin/sh


echo "=== 启动 QMS (Docker 模式) ==="

# 设置环境变量
export DOCKER=1

if [ -n "$GPID" ]; then
    echo "使用GPID: $GPID"
    export GPID
else
    echo "未设置GPID环境变量，使用默认值"
fi

if [ -n "$GUID" ]; then
    echo "使用GUID: $GUID"
    export GUID
else
    echo "未设置GUID环境变量，使用默认值"
fi

# 创建用户和组
setup_user_and_group() {
    if [ -n "$GPID" ]; then
        if ! getent group "$GPID" >/dev/null 2>&1; then
            echo "组 $GPID 不存在，创建组..."
            addgroup -g "$GPID" "$GPID" 2>/dev/null || addgroup "$GPID"
            echo "组 $GPID 创建完成"
        else
            echo "组 $GPID 已存在"
        fi
    fi

    if [ -n "$GUID" ]; then
        if ! id "$GUID" >/dev/null 2>&1; then
            echo "用户 $GUID 不存在，创建用户..."
            USER_GROUP=""
            if [ -n "$GPID" ]; then
                USER_GROUP="-G $GPID"
            fi
            adduser -u "$GUID" $USER_GROUP -D "$GUID" 2>/dev/null || adduser -D "$GUID"
            echo "用户 $GUID 创建完成"
        else
            echo "用户 $GUID 已存在"
        fi
    fi
}

setup_user_and_group

# 启动文件监视
echo "启动文件更新监视器..."
/app/scripts/watch_update.sh &
WATCH_PID=$!
cd /app

handle_signal() {
    echo "收到关闭信号，转发给主进程..."
    if [ -n "$MAIN_PID" ] && kill -0 "$MAIN_PID" >/dev/null 2>&1; then
        kill -TERM "$MAIN_PID"
        wait "$MAIN_PID"
    fi
    if [ -n "$WATCH_PID" ] && kill -0 "$WATCH_PID" >/dev/null 2>&1; then
        kill -TERM "$WATCH_PID"
        wait "$WATCH_PID"
    fi
    exit 0
}

trap 'handle_signal' INT TERM

# 主循环，确保可以多次更新
while true; do
    # 启动主进程，支持GPID和GUID环境变量
    if [ -n "$GUID" ]; then
        echo "使用GUID=$GUID 启动主程序"
        if id "$GUID" >/dev/null 2>&1; then
            echo "切换到用户 $GUID 并启动主程序"
            su - "$GUID" -c "/app/QMediaSync --guid $GUID &"
        else
            echo "用户 $GUID 不存在，直接启动主程序"
            /app/QMediaSync &
        fi
    else
        echo "未设置GUID，使用默认参数启动主程序"
        /app/QMediaSync &
    fi
    MAIN_PID=$!
    echo "主进程ID: $MAIN_PID"

    # 等待主进程退出
    wait $MAIN_PID
    echo "主进程退出，等待更新完成..."

    # 如果主进程退出，检查是否有更新
    if [ -f "/app/qms.update.tar.gz" ]; then
        echo "主进程退出，检测到新版本，执行更新..."
        if [ -d "/app/update" ]; then
            rm -rf /app/update
            echo "旧版本更新目录已删除"
        fi
        mkdir /app/update
        echo "创建新版本更新目录 /app/update"
        # 解压更新文件
        echo "解压更新文件..."
        tar -zxvf /app/qms.update.tar.gz -C /app/update
        echo "更新文件已解压到 /app/update"
        # 检查/app/old是否存在，存在则删除，不存在则创建
        if [ -d "/app/old" ]; then
            rm -rf /app/old
            echo "旧版本目录已删除"
        fi
        mkdir /app/old
        echo "创建备份目录 /app/old"
        echo "备份旧版本..."
        # 备份旧版本
        mv /app/QMediaSync /app/old/QMediaSync
        mv /app/web_statics /app/old/web_statics
        mv /app/scripts /app/old/scripts
        # 替换新版本
        mv /app/update/QMediaSync /app/QMediaSync
        mv /app/update/web_statics /app/web_statics
        mv /app/update/scripts /app/scripts
        chmod +x /app/QMediaSync
        chmod +x /app/scripts/*.sh
        # 删除压缩包
        rm -f /app/qms.update.tar.gz
        echo "更新压缩包已删除"
        echo "更新完成，准备重启主进程..."
        # 继续循环，重启主进程
    else
        echo "主进程退出，未检测到更新文件，退出容器..."
        exit 0
    fi
done