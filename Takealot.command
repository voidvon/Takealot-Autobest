#!/bin/bash
DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
cd "$DIR"

echo "========================================================"
echo "🚀 正在启动 Takealot 自动化改价控制中心 (macOS 版)"
echo "========================================================"

if [ -f "./Takealot-mac" ]; then
    ./Takealot-mac
elif [ -f "./Takealot.app/Contents/MacOS/Takealot" ]; then
    ./Takealot.app/Contents/MacOS/Takealot
elif [ -f "./takealot" ]; then
    ./takealot
else
    echo "❌ 错误: 未找到可执行程序文件"
    read -p "按回车键退出..."
fi
