#!/bin/bash

# Navigate to the script directory
DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null 2>&1 && pwd )"
cd "$DIR"

# Build binary if not already built or if sources are newer
if [ ! -f "takealot" ]; then
    echo "🔨 首次运行，正在编译 Go 原生单文件二进制..."
    go build -o takealot .
fi

# Execute the Go binary
./takealot "$@"
