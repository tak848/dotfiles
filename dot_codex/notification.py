#!/usr/bin/env -S uv run --script
# /// script
# requires-python = "==3.13.5"
# dependencies = [
# ]
# ///

import json
import platform
import re
import subprocess
import sys

# sample
# {
#   "type": "agent-turn-complete",
#   "turn-id": "12345",
#   "input-messages": ["Rename `foo` to `bar` and update the callsites."],
#   "last-assistant-message": "Rename complete and verified `cargo build` succeeds."
# }


def main() -> int:
    try:
        input_data = json.load(sys.stdin)
    except json.JSONDecodeError as e:
        print(f"Error: Invalid JSON input: {e}", file=sys.stderr)
        return 1

    if input_data.get("type") == "agent-turn-complete":
        return agent_turn_complete(input_data)

    return 0


def agent_turn_complete(input_data: dict) -> int:
    assistant_message = input_data.get("last-assistant-message")
    if assistant_message and platform.system() == "Darwin":
        subprocess.run(
            ["say", assistant_message],
            check=False,  # エラーが発生してもスクリプトは続行
            capture_output=True,  # 出力をキャプチャしてコンソールに表示しない
        )

    return 0


if __name__ == "__main__":
    sys.exit(main())

# print(input_data)

# # macOSの場合、通知メッセージを音声で読み上げ
# if platform.system() == "Darwin" and "message" in input_data:
#     try:
#         # メッセージをそのまま読み上げ
#         message = input_data.get("message")
#         if not message:
#             title = input_data.get("title")
#             if title:
#                 message = title
#             else:
#                 sys.exit(0)

#         # ASCII文字のみかチェック
#         is_ascii = all(ord(char) < 128 for char in message)

#         # ASCII文字のみなら英語音声、それ以外は日本語音声を使用
#         if is_ascii:
#             subprocess.run(
#                 ["say", message],
#                 check=False,  # エラーが発生してもスクリプトは続行
#                 capture_output=True,  # 出力をキャプチャしてコンソールに表示しない
#             )
#         else:
#             subprocess.run(
#                 ["say", "-v", "Kyoko", message],
#                 check=False,  # エラーが発生してもスクリプトは続行
#                 capture_output=True,  # 出力をキャプチャしてコンソールに表示しない
#             )
#     except Exception as e:
#         # sayコマンドが失敗してもスクリプト全体は正常終了
#         print(f"Warning: Failed to play audio notification: {e}", file=sys.stderr)
