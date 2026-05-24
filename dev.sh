#!/bin/bash
# 快速开发构建脚本

set -e

# 颜色定义
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m' # No Color

echo -e "${GREEN}🔨 构建中...${NC}"
go build -o ./bin/cockpit ./cmd/cockpit

if [ $? -eq 0 ]; then
    echo -e "${GREEN}✅ 构建成功${NC}"
    echo -e "${YELLOW}📦 输出: ./bin/cockpit${NC}"

    # 检查是否正在运行
    if pgrep -x "cockpit" > /dev/null; then
        echo -e "${YELLOW}⚠️  检测到 cockpit 正在运行，停止旧进程...${NC}"
        pkill -x "cockpit" || true
        sleep 1
    fi

    echo -e "${GREEN}🚀 启动服务...${NC}"
    ./bin/cockpit
else
    echo -e "${RED}❌ 构建失败${NC}"
    exit 1
fi
