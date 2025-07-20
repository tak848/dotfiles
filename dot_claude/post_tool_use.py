#!/usr/bin/env -S uv run --script
# /// script
# requires-python = "==3.13.5"
# dependencies = [
# ]
# ///

import json
import re
import sys

# sample
# {
#   "session_id": "abc123",
#   "transcript_path": "~/.claude/projects/.../00893aaf-19fa-41d2-8238-13269b9b3ca0.jsonl",
#   "tool_name": "Write",
#   "tool_input": {
#     "file_path": "/path/to/file.txt",
#     "content": "file content"
#   },
#   "tool_response": {
#     "filePath": "/path/to/file.txt",
#     "success": true
#   }
# }

try:
    input_data = json.load(sys.stdin)
except json.JSONDecodeError as e:
    print(f"Error: Invalid JSON input: {e}", file=sys.stderr)
    sys.exit(1)

# print(input_data)

# ファイル編集ツールが実行された場合、最終行に空行を追加
tool_name = input_data.get("tool_name", "")
if tool_name in ["Write", "Edit", "MultiEdit"]:
    # ツールの実行が成功した場合のみ処理
    tool_response = input_data.get("tool_response", {})
    if tool_response.get("success", False):
        # ファイルパスを取得
        file_path = input_data.get("tool_input", {}).get("file_path", "")
        if file_path:
            try:
                # ファイルを読み込み
                with open(file_path, "r", encoding="utf-8") as f:
                    content = f.read()
                
                # 最終行に改行がない場合は追加
                if content and not content.endswith("\n"):
                    with open(file_path, "a", encoding="utf-8") as f:
                        f.write("\n")
                    print(f"Added newline to end of file: {file_path}", file=sys.stderr)
            except Exception as e:
                # エラーが発生しても処理を続行
                print(f"Warning: Failed to add newline to {file_path}: {e}", file=sys.stderr)
