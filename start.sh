#!/bin/bash
set -euo pipefail

# Navigate to the script directory
DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null 2>&1 && pwd )"
cd "$DIR"
OUTPUT_DIR="$DIR/dist"
BINARY="$OUTPUT_DIR/takealot"

# Build binary if not already built
if [ ! -f "$BINARY" ]; then
    echo "🔨 首次运行，正在编译 Go 原生单文件二进制..."
    mkdir -p "$OUTPUT_DIR"
    go build -o "$BINARY" .
fi

# Execute the Go binary
exec "$BINARY" "$@"
