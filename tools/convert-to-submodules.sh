#!/usr/bin/env bash
# tools/submodules-init-min.sh
# 极简“submodules init”：就地注册已存在的嵌套仓库为子模块，不做额外动作。
# - 发现所有子仓(*/.git/config)
# - 读取 remote.origin.url
# - 若路径已被索引跟踪但不是 submodule：先从索引剔除，再注册
# - 使用 `git submodule absorbgitdirs` 吸收内部 .git 到主仓
# - 默认自动提交（可用 AUTO_COMMIT=0 关闭）
set -euo pipefail

AUTO_COMMIT="${AUTO_COMMIT:-1}"   # 1=自动提交；0=仅暂存不提交
DRY_RUN="${DRY_RUN:-0}"           # 1=仅打印要做的事，不执行

# 收集所有嵌套仓库路径与 URL（排除主仓 .git/config）
mapfile -t ENTRIES < <(
  find . -type f -path '*/.git/config' ! -path './.git/config' -print0 \
  | xargs -0 -I{} sh -c '
      d=$(dirname "$(dirname "{}")");
      u=$(git -C "$d" config --get remote.origin.url || true);
      echo "${d#./}|||$u"
    '
)

# 判断 path 是否已在 .gitmodules 注册为 submodule
is_submodule_path() {
  git config -f .gitmodules --get-regexp 'submodule\..*\.path' 2>/dev/null \
    | awk "{print \$2}" | grep -qx "$1"
}

# 判断 path 是否已被索引跟踪（普通文件/目录或 gitlink）
is_tracked_in_index() {
  git ls-files --error-unmatch "$1" >/dev/null 2>&1
}

for e in "${ENTRIES[@]}"; do
  path="${e%%|||*}"
  url="${e##*|||}"

  # 跳过没有 URL 的路径（不应该出现，但以防万一）
  if [[ -z "${url}" ]]; then
    echo "Skip (no remote url): ${path}"
    continue
  fi

  # 已是 submodule 则跳过
  if is_submodule_path "$path"; then
    echo "Skip (already submodule): ${path}"
    continue
  fi

  echo "Register submodule: ${path}  <-  ${url}"

  if [[ "$DRY_RUN" == "1" ]]; then
    echo "  DRY_RUN: would run: git rm -r --cached '${path}' (if tracked)"
    echo "  DRY_RUN: would run: git submodule add -f '${url}' '${path}'"
    echo "  DRY_RUN: would run: git submodule absorbgitdirs -- '${path}'"
    echo "  DRY_RUN: would run: git add .gitmodules '${path}'"
    continue
  fi

  # 若该路径已在索引中，但不是 submodule（普通目录/文件），先从索引剔除，避免：
  # "already exists in the index and is not a submodule"
  if is_tracked_in_index "$path"; then
    # 如果是 gitlink（子模块指针），git rm --cached 也安全；不是则可去除普通索引
    git rm -r --cached "$path" >/dev/null 2>&1 || true
  fi

  # 就地注册为 submodule；对已存在的工作区，Git 会显示 "Adding existing repo..."
  git submodule add -f "$url" "$path"

  # 吸收内部 .git 到主仓 .git/modules（官方推荐做法）
  if [[ -d "$path/.git" ]]; then
    git submodule absorbgitdirs -- "$path"
  fi

  # 暂存变更
  git add .gitmodules "$path"
done

# 统一提交
if [[ "$DRY_RUN" == "0" && "$AUTO_COMMIT" == "1" ]]; then
  if ! git diff --cached --quiet; then
    git commit -m "chore(submodule): register existing nested repos as submodules"
    echo "Commit created."
  else
    echo "No staged changes."
  fi
fi

echo "Done. Verify with:"
echo "  git submodule status"
echo "  cat .gitmodules"
echo "  git ls-files -t | grep '^S ' || echo 'no gitlinks'"
