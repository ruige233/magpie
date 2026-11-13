#!/bin/sh
# 同步上游 yetone/magpie 到自己的 fork ruige233/magpie。
# 自己的改动始终是 main 上最新的一层（rebase），所以历史干净、随时能跟上游走。
#
#   ./sync-magpie-upstream.sh         # 同步：fetch → rebase → 构建校验 → 强推 fork
#   ./sync-magpie-upstream.sh --dry   # 只看差多少，不动历史
#
# 脚本就放在 magpie 仓库根目录，跟仓库一起走。
#
# 冲突时：改完文件 → git add <file> → git rebase --continue
# 想放弃：git rebase --abort
set -eu
cd "$(dirname "$0")"
export PATH=/opt/homebrew/bin:$PATH
export GOPROXY=${GOPROXY:-https://goproxy.cn,direct}

git fetch upstream --prune
git fetch origin --prune

if [ "${1:-}" = "--dry" ]; then
  echo "落后上游: $(git rev-list --count main..upstream/main) 个提交"
  echo "自己的:   $(git rev-list --count upstream/main..main) 个提交"
  git log --oneline main..upstream/main | head -20
  exit 0
fi

if [ -n "$(git status --porcelain)" ]; then
  echo "工作区不干净，先 commit 或 git stash：" >&2
  git status --short >&2
  exit 1
fi

if git merge-base --is-ancestor upstream/main main; then
  echo "已包含上游全部提交，无需同步。"
  exit 0
fi

echo "== 变基到 upstream/main ($(git log --oneline -1 upstream/main)) =="
git rebase upstream/main || { echo "变基冲突：解决后 git add + git rebase --continue；放弃用 git rebase --abort" >&2; exit 1; }

echo "== 构建校验 =="
env -u DSH_HOME go build ./... || { echo "构建失败：git reset --hard ORIG_HEAD 可回到变基前" >&2; exit 1; }

echo "== 推到 fork =="
git push --force-with-lease origin main
git log --oneline -3
