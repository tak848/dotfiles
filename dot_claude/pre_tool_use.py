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
#   }
# }

try:
    input_data = json.load(sys.stdin)
except json.JSONDecodeError as e:
    print(f"Error: Invalid JSON input: {e}", file=sys.stderr)
    sys.exit(1)

# print(input_data)
