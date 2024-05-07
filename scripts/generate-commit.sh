#!/bin/sh
echo "Start to generate commit message"

# 获取当前 commit 的哈希
current_commit=$(git rev-parse HEAD)
echo "Current Commit: $current_commit"

# 获取不包含 'alpha' 和 'beta'，且不是当前 commit 的最近一个 tag
previous_tag=$(git for-each-ref --sort=-creatordate --format '%(refname:short)' refs/tags | grep -v '\-alpha' | grep -v '\-beta' | while read tag; do
    tag_commit=$(git rev-list -n 1 $tag)
    if [ "$tag_commit" != "$current_commit" ]; then
        echo $tag
        break
    fi
done | head -n 1)

echo "Previous Tag: $previous_tag"

# 检查 previous_tag 是否为空
if [ -z "$previous_tag" ]; then
    echo "No valid previous tag found that is not the current commit."
    exit 1
fi

echo "Start to check commit message"
# 检查/tmp/raw_commit_message.txt的行数
line_count=$(wc -l < /tmp/raw_commit_message.txt)

if [ "$line_count" -le 3 ]; then
    echo "Fewer than 4 commit messages found. Fetching the last 30 commits:"
    git log -n 30 --format="%s" > /tmp/raw_commit_message.txt
fi

cat /tmp/raw_commit_message.txt