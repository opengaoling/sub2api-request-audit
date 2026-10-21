# Sub2API Request Audit

> Sub2API をベースにメンテナンスされた AI API ゲートウェイプロジェクト。本番環境の可用性、リクエスト監査、アップストリーム管理、複数プロトコル転送、バックエンド運用に重点を置いています。

## 🚀 ワンクリックデプロイ (推奨)

Docker 環境の自動セットアップ、PostgreSQL 18 & Redis 8 の構成、共有 Caddy による SSL 証明書の自動取得・更新に対応：

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/opengaoling/sub2api-request-audit/main/deploy.sh)
```

パラメータを指定したサイレントデプロイにも対応：

```bash
# ドメインをバインドし自動 HTTPS を有効化 (推奨)
bash <(curl -fsSL https://raw.githubusercontent.com/opengaoling/sub2api-request-audit/main/deploy.sh) -d api.yourdomain.com -e admin@yourdomain.com -y

# ドメインなしの純粋な HTTP ポートデプロイ (デフォルトポート: 8080)
bash <(curl -fsSL https://raw.githubusercontent.com/opengaoling/sub2api-request-audit/main/deploy.sh) -p 8080 -y
```

## 🛠️ 管理コマンド

デプロイ完了後、サーバー上の任意の場所でグローバルコマンド `sub2api` を使用できます：

| コマンド | 説明 |
| --- | --- |
| `sub2api` | インタラクティブ管理コンソールを開く |
| `sub2api status` | コンテナサービスのステータス確認 |
| `sub2api restart` | サービス再起動 |
| `sub2api update` | 最新イメージを取得して無停止更新 |
| `sub2api logs` | Sub2API メインサービスのリアルタイムログ |
| `sub2api logs-caddy` | Caddy アクセスおよび SSL 証明書ログ |
| `sub2api info` | 現在の設定、アクセス先、認証情報を確認 |
| `sub2api clean` | システムメンテ（キャッシュ削除/ログ切り詰め/完全アンインストール） |

## 🐳 Docker Compose による手動デプロイ

```bash
git clone https://github.com/opengaoling/sub2api-request-audit.git
cd sub2api-request-audit/deploy
cp .env.example .env  # パスワードやシークレットキーを必要に応じて変更
docker compose -f docker-compose.local.yml up -d
```

サービス起動後、`http://<サーバーIP>:8080` にアクセスして初期設定を行います。

## 📄 ライセンス

本プロジェクトは [GNU Lesser General Public License v3.0](LICENSE) に準拠しています。
