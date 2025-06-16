# ========================================
# ローカルZsh関数・エイリアスの例
# ========================================
# このディレクトリ内のファイルはGit管理されません
# 必要に応じて新しいファイルを作成してください

# 例: プロジェクト固有のエイリアス
# alias myproject="cd ~/work/myproject && docker-compose up -d"

# 例: ローカル環境用の関数
# function deploy-local() {
#     echo "Deploying to local environment..."
#     # デプロイスクリプトの実行
# }

# 例: 会社固有のツール
# function company-vpn() {
#     case "$1" in
#         start)
#             echo "Starting company VPN..."
#             # VPN開始コマンド
#             ;;
#         stop)
#             echo "Stopping company VPN..."
#             # VPN停止コマンド
#             ;;
#         *)
#             echo "Usage: company-vpn {start|stop}"
#             ;;
#     esac
# }