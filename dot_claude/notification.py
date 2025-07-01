#!/usr/bin/env python3
import json
import re
import sys
import platform
import subprocess

# sample
# {
#   "session_id": "abc123",
#   "transcript_path": "~/.claude/projects/.../00893aaf-19fa-41d2-8238-13269b9b3ca0.jsonl",
#   "message": "Task completed successfully",
#   "title": "Claude Code"
# }

try:
    input_data = json.load(sys.stdin)
except json.JSONDecodeError as e:
    print(f"Error: Invalid JSON input: {e}", file=sys.stderr)
    sys.exit(1)

print(input_data)

# macOSの場合、通知メッセージを音声で読み上げ
if platform.system() == "Darwin" and "message" in input_data:
    try:
        # メッセージをそのまま読み上げ
        message = input_data.get("message")
        if not message:
            title = input_data.get("title")
            if title:
                message = title
            else:
                sys.exit(0)
        
        # ASCII文字のみかチェック
        is_ascii = all(ord(char) < 128 for char in message)
        
        # ASCII文字のみなら英語音声、それ以外は日本語音声を使用
        if is_ascii:
            subprocess.run(
                ["say", message],
                check=False,  # エラーが発生してもスクリプトは続行
                capture_output=True  # 出力をキャプチャしてコンソールに表示しない
            )
        else:
            subprocess.run(
                ["say", "-v", "Kyoko", message],
                check=False,  # エラーが発生してもスクリプトは続行
                capture_output=True  # 出力をキャプチャしてコンソールに表示しない
            )
    except Exception as e:
        # sayコマンドが失敗してもスクリプト全体は正常終了
        print(f"Warning: Failed to play audio notification: {e}", file=sys.stderr)
