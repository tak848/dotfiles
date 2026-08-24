# snowx: PYTHONPATH を外して Snowflake CLI (snow) を実行する
#
# snow は起動時に google.protobuf を import する。google は名前空間パッケージ
# （__init__.py を持たず、複数の場所に散ったツリーがマージされる）で、Python の
# sys.path 探索順は「スクリプトのディレクトリ → PYTHONPATH → venv の site-packages」。
# PYTHONPATH 側に __init__.py を持つ通常パッケージとしての google/ があると、Python は
# そこで google の解決を打ち切り、venv 側の google/protobuf に到達できずに
# ModuleNotFoundError: No module named 'google.protobuf' で落ちる。
# protoc / gRPC の生成コード置き場を .envrc で PYTHONPATH に入れている repo で踏む。
#
# snow 自体は PYTHONPATH を必要としないので、外して起動すれば済む。素の snow は
# 残してあるので、PYTHONPATH を汚していない場所ではそのまま使える。
#
# 注意:
#   - PYTHONPATH= （空文字）では駄目。空エントリは cwd に解決されうるので unset する
#   - 呼び出し元シェルの PYTHONPATH は触らない。repo 側の Python 作業を壊さないため
#   - env は PATH から実行ファイルだけを探すので、この関数に再帰することはない
#   - zsh 関数なのでインタラクティブシェルでのみ有効。スクリプトや他ツールが直接 snow を
#     呼ぶ経路では効かない（codexp と同じトレードオフ）

snowx() {
    if ! command -v snow &> /dev/null; then
        echo "snowx: snow が見つかりません。mise install で snowflake-cli を入れてください。" >&2
        return 127
    fi

    env -u PYTHONPATH snow "$@"
}
