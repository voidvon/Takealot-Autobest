#!/bin/bash
DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
cd "$DIR"

if [ -d "./Takealot.app" ]; then
    open "./Takealot.app"
elif [ -d "./build/bin/Takealot.app" ]; then
    open "./build/bin/Takealot.app"
elif [ -f "./Takealot-mac" ]; then
    open "./Takealot-mac"
elif [ -f "./takealot" ]; then
    ./takealot
else
    echo "❌ 错误: 未找到可执行程序文件"
    read -p "按回车键退出..."
fi
