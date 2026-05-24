#!/bin/bash
# 快速开发构建脚本

set -e

# 颜色定义
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# 解析参数
ADDR="${ADDR:-0.0.0.0:9000}"
DATADIR="${DATADIR:-./data}"

while [[ $# -gt 0 ]]; do
  case $1 in
    -addr)
      ADDR="$2"
      shift 2
      ;;
    -data)
      DATADIR="$2"
      shift 2
      ;;
    *)
      echo -e "${YELLOW}未知选项: $1${NC}"
      exit 1
      ;;
  esac
done

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
    echo -e "${BLUE}地址: http://$ADDR${NC}"
    echo -e "${BLUE}数据目录: $DATADIR${NC}"
    echo ""

    exec ./bin/cockpit -addr "$ADDR" -data "$DATADIR"
else
    echo -e "${RED}❌ 构建失败${NC}"
    exit 1
fi
