#!/usr/bin/env bash
# 构建单文件分发版:前端产物与 skills 内嵌进二进制,交叉编译 Windows/Linux(amd64/arm64)。
# 用法:scripts/build-release.sh
#   可选:GBI_TARGETS="linux/amd64,windows/arm64" scripts/build-release.sh 仅构建指定目标
#
# 产物(dist/):
#   gbi-windows-amd64.exe  gbi-windows-arm64.exe
#   gbi-linux-amd64        gbi-linux-arm64
#
# 重要:内嵌源目录(staging)只在构建期间存在,构建结束(含失败/中断)会自动清空。
# 这样普通裸 `go build` 不会把上一次构建的陈旧前端静默编进二进制——裸编译产物会
# 在启动时因缺少内嵌 index.html 而显式失败,并提示改用本脚本。
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

cleanup_staging() {
  # 仅保留 .gitkeep;清掉构建期间拷入的前端/skills,防止陈旧副本被后续裸构建复用
  find server/internal/assets/dist -mindepth 1 ! -name .gitkeep -exec rm -rf {} + 2>/dev/null || true
  find server/internal/assets/skills -mindepth 1 ! -name .gitkeep -exec rm -rf {} + 2>/dev/null || true
}
trap cleanup_staging EXIT

# 清掉上一次中断可能残留的 staging 内容
cleanup_staging

# 目标清单:默认四平台;可用 GBI_TARGETS(逗号或空格分隔的 os/arch)覆盖
TARGETS="${GBI_TARGETS:-windows/amd64 windows/arm64 linux/amd64 linux/arm64}"
TARGETS="${TARGETS//,/ }"
if [ -z "${TARGETS// /}" ]; then
  echo "错误:GBI_TARGETS 为空,应形如 linux/amd64,windows/arm64" >&2
  exit 1
fi

# 先校验目标,避免目标写错时白跑一次前端构建
TARGET_LIST=()
for target in $TARGETS; do
  if [[ ! "$target" =~ ^[a-z0-9]+/[a-z0-9]+$ ]]; then
    echo "错误:目标格式应为 os/arch(例如 linux/amd64),收到:$target" >&2
    exit 1
  fi
  TARGET_LIST+=("$target")
done

echo "==> 构建前端"
(cd web && npm run build)

echo "==> 拷贝前端产物到内嵌目录"
cp -r web/dist/. server/internal/assets/dist/
if [ ! -f server/internal/assets/dist/index.html ]; then
  echo "错误:web/dist/index.html 缺失,前端构建失败" >&2
  exit 1
fi

echo "==> 拷贝 skills 到内嵌目录"
cp -r server/skills/. server/internal/assets/skills/
if [ ! -f server/internal/assets/skills/text2sql/SKILL.md ]; then
  echo "错误:server/skills/text2sql/SKILL.md 缺失" >&2
  exit 1
fi

mkdir -p dist

BUILT=()
for target in "${TARGET_LIST[@]}"; do
  target_os="${target%%/*}"
  target_arch="${target##*/}"

  out="dist/gbi-${target_os}-${target_arch}"
  if [ "$target_os" = "windows" ]; then
    out="${out}.exe"
  fi

  echo "==> 编译 $target_os/$target_arch"
  # 先删旧产物,避免编译失败时残留旧二进制被误当作本次构建结果
  rm -f "$out"
  (cd server && CGO_ENABLED=0 GOOS="$target_os" GOARCH="$target_arch" \
    go build -trimpath -ldflags "-s -w" -o "../$out" ./cmd/gbi)
  if [ ! -s "$out" ]; then
    echo "错误:编译产物缺失或为空:$out" >&2
    exit 1
  fi
  if [ "$target_os" != "windows" ]; then
    chmod +x "$out" 2>/dev/null || true
  fi
  ls -lh "$out"
  BUILT+=("$ROOT/$out")
done

echo
echo "完成,产物:"
printf '  %s\n' "${BUILT[@]}"
echo "提示:分发请使用上述产物;裸 go build 不刷新内嵌前端,不可用于分发。"
