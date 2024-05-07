#!/bin/sh
echo "Start to generate commit message"

previous_commit=$(git for-each-ref --sort=-creatordate --format '%(refname:short)' refs/tags | grep -v '\-alpha' | grep -v '\-beta' | head -n 1)
echo "Previous Commit: $previous_commit"
current_commit=$(git rev-parse HEAD)
echo "Current Commit: $current_commit"
git log --format="%s" "$previous_commit".. "$current_commit"> /tmp/raw_commit_message.txt && cat /tmp/raw_commit_message.txt

echo "Start to check commit message"
# 检查/tmp/raw_commit_message.txt的行数
line_count=$(wc -l < /tmp/raw_commit_message.txt)

if [ "$line_count" -le 3 ]; then
    echo "Fewer than 4 commit messages found. Fetching the last 30 commits:"
    git log -n 30 --format="%s" > /tmp/raw_commit_message.txt
fi

cat /tmp/raw_commit_message.txt