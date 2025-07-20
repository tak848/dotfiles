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

# ファイル編集ツールの場合、最終行に空行を確保
if input_data.get("tool_name") in ["Write", "Edit", "MultiEdit"]:
    # ツールが成功した場合のみ処理
    tool_response = input_data.get("tool_response", {})
    if tool_response.get("success", False):
        # ファイルパスを取得
        file_path = None
        tool_input = input_data.get("tool_input", {})
        
        if input_data["tool_name"] in ["Write", "Edit", "MultiEdit"]:
            file_path = tool_input.get("file_path")
        
        if file_path:
            try:
                # ファイルを読み込む
                with open(file_path, 'r', encoding='utf-8') as f:
                    content = f.read()
                
                # 最終行に空行がない場合は追加
                if content and not content.endswith('\n'):
                    with open(file_path, 'a', encoding='utf-8') as f:
                        f.write('\n')
                    print(f"Added newline at end of {file_path}", file=sys.stderr)
            except Exception as e:
                # エラーが発生しても処理を継続（ファイルが存在しない場合など）
                print(f"Warning: Could not check/add newline to {file_path}: {e}", file=sys.stderr)
