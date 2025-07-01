#!/usr/bin/env python3
import json
import re
import sys
import os
import platform
import subprocess

# sample
# {
#   "session_id": "abc123",
#   "transcript_path": "~/.claude/projects/.../00893aaf-19fa-41d2-8238-13269b9b3ca0.jsonl",
#   "stop_hook_active": true
# }

try:
    input_data = json.load(sys.stdin)
except json.JSONDecodeError as e:
    print(f"Error: Invalid JSON input: {e}", file=sys.stderr)
    sys.exit(1)

# print(input_data)
# output pwd
print(f"pwd: {os.getcwd()}")

# macOSの場合、音声で通知
if platform.system() == "Darwin":
    try:
        # 日本語音声（Kyoko）で読み上げ
        subprocess.run(
            ["say", "-v", "Kyoko", "Claudeセッションが終了しました"],
            check=False,  # エラーが発生してもスクリプトは続行
            capture_output=True  # 出力をキャプチャしてコンソールに表示しない
        )
    except Exception as e:
        # sayコマンドが失敗してもスクリプト全体は正常終了
        print(f"Warning: Failed to play audio notification: {e}", file=sys.stderr)
