#!/usr/bin/env bash
set -euo pipefail

# 要转换为子模块的路径列表（相对主仓根）
MODULES=(
  "opentelemetry-collector"
  "agents/process-exporter"
  "agents/node_exporter"
  "agents/deepflow"
  "agents/vector"
  "openobserve"
)

# 1) 主仓必须干净
if [[ -n "$(git status --porcelain)" ]]; then
  echo "ERROR: main repo has uncommitted changes. Commit/stash first."
  exit 1
fi

# 2) 逐个转换
for P in "${MODULES[@]}"; do
  echo "==> Converting $P to submodule"

  if [[ ! -d "$P/.git" ]]; then
    echo "  Skip: $P is not a standalone git repo (no $P/.git)"
    continue
  fi

  # 2.1 读取子仓库 URL 与当前 SHA
  URL=$(git -C "$P" config --get remote.origin.url || true)
  SHA=$(git -C "$P" rev-parse HEAD || true)
  if [[ -z "$URL" || -z "$SHA" ]]; then
    echo "ERROR: cannot get URL/SHA for $P"; exit 2
  fi
  echo "  origin: $URL"
  echo "  pinned: $SHA"

  # 2.2 子仓必须已 push（否则克隆不到该 commit）
  if ! git -C "$P" ls-remote --exit-code "$URL" "$SHA" >/dev/null 2>&1; then
    echo "ERROR: commit $SHA not found on remote for $P. Push it first:"
    echo "  (cd $P && git push origin HEAD)"
    exit 3
  fi

  # 2.3 子仓工作区干净（避免丢改动）
  if [[ -n "$(git -C "$P" status --porcelain)" ]]; then
    echo "ERROR: $P has uncommitted changes. Commit/push or stash, then retry."
    exit 4
  fi

  # 2.4 若主仓曾跟踪此目录，先从索引移除（不删文件）
  git rm -r --cached "$P" >/dev/null 2>&1 || true

  # 2.5 备份目录（防止 git submodule add 要求空目录）
  TMP="$P.__backup_before_submodule__"
  if [[ -e "$TMP" ]]; then rm -rf "$TMP"; fi
  mv "$P" "$TMP"

  # 2.6 添加子模块（克隆到原路径）
  git submodule add "$URL" "$P"

  # 2.7 切换到固定的 SHA（用本地分支 pin，更直观）
  ( cd "$P" && git fetch --all --tags && git checkout -B _pinned "$SHA" )

  # 2.8 清理备份
  rm -rf "$TMP"

  # 2.9 记录变更（一次性全部提交也可，视你习惯）
  git add .gitmodules "$P"
done

# 3) 统一提交
git commit -m "chore(submodule): convert embedded repos to submodules pinned to current commits"
echo "Done. Next: git submodule status"
