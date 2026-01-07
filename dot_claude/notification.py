#!/usr/bin/env -S uv run --script
# /// script
# requires-python = "==3.13.5"
# dependencies = []
# ///

import base64
import hashlib
import json
import os
import platform
import subprocess
import sys
from pathlib import Path

# sample
# {
#   "session_id": "abc123",
#   "transcript_path": "~/.claude/projects/.../00893aaf-19fa-41d2-8238-13269b9b3ca0.jsonl",
#   "message": "Task completed successfully",
#   "title": "Claude Code"
# }

CACHE_DIR = Path.home() / ".cache" / "gtts"
VOICE_JA = "ja-JP-Neural2-B"
VOICE_EN = "en-US-Neural2-D"
SPEED = 1.0


def get_audio_player() -> list[str] | None:
    """利用可能なオーディオプレイヤーを検出"""
    players = [
        ["mpv", "--no-terminal", "--no-video"],
        ["afplay"],
        ["play", "-q"],
    ]
    for player in players:
        try:
            subprocess.run(
                ["which", player[0]],
                check=True,
                capture_output=True,
            )
            return player
        except subprocess.CalledProcessError:
            continue
    return None


def speak_gtts(message: str) -> None:
    """Google Cloud TTS で音声合成・再生（キャッシュ付き）"""
    api_key = os.environ.get("GOOGLE_API_KEY")
    if not api_key:
        # フォールバック: sayコマンド
        subprocess.run(["say", message], check=False, capture_output=True)
        return

    # 言語判定: ASCII のみなら英語
    is_ascii = all(ord(c) < 128 for c in message)
    voice = VOICE_EN if is_ascii else VOICE_JA
    lang_code = voice[:5]  # "ja-JP" or "en-US"

    # キャッシュキー生成
    cache_key = f"{message}|{voice}|{SPEED}"
    cache_hash = hashlib.md5(cache_key.encode()).hexdigest()
    CACHE_DIR.mkdir(parents=True, exist_ok=True)
    cache_file = CACHE_DIR / f"{cache_hash}.mp3"

    # キャッシュがあれば即再生
    if cache_file.exists():
        player = get_audio_player()
        if player:
            subprocess.run([*player, str(cache_file)], check=False, capture_output=True)
        return

    # API リクエスト
    import urllib.request

    url = "https://texttospeech.googleapis.com/v1/text:synthesize"
    payload = json.dumps(
        {
            "input": {"text": message},
            "voice": {"languageCode": lang_code, "name": voice},
            "audioConfig": {"audioEncoding": "MP3", "speakingRate": SPEED},
        }
    ).encode()

    req = urllib.request.Request(
        url,
        data=payload,
        headers={
            "X-Goog-Api-Key": api_key,
            "Content-Type": "application/json",
        },
    )

    try:
        with urllib.request.urlopen(req, timeout=10) as resp:
            data = json.loads(resp.read())
            audio_content = base64.b64decode(data["audioContent"])
            cache_file.write_bytes(audio_content)
    except Exception as e:
        print(f"Warning: TTS API failed: {e}", file=sys.stderr)
        # フォールバック
        subprocess.run(["say", message], check=False, capture_output=True)
        return

    # 再生
    player = get_audio_player()
    if player:
        subprocess.run([*player, str(cache_file)], check=False, capture_output=True)


def main():
    try:
        input_data = json.load(sys.stdin)
    except json.JSONDecodeError as e:
        print(f"Error: Invalid JSON input: {e}", file=sys.stderr)
        sys.exit(1)

    print(input_data)

    # macOS/Linuxの場合、通知メッセージを音声で読み上げ
    if platform.system() in ("Darwin", "Linux") and "message" in input_data:
        try:
            message = input_data.get("message")
            if not message:
                title = input_data.get("title")
                if title:
                    message = title
                else:
                    sys.exit(0)

            speak_gtts(message)

        except Exception as e:
            print(f"Warning: Failed to play audio notification: {e}", file=sys.stderr)


if __name__ == "__main__":
    main()
