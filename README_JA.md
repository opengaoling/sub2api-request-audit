# Sub2API Request Audit

Sub2API Request Audit は、Sub2API をベースに保守している AI API ゲートウェイです。上流アカウント、API キー、リクエスト転送、スケジューリング、課金、監査、管理画面を統合して扱います。

この README には、スポンサー、提携広告、招待リンク、紹介報酬リンク、トレンド表示リンクを含めません。

## 注意事項

- 上流 AI サービスを利用する前に、各サービスの利用規約と地域の法令を確認してください。
- 本プロジェクトは技術実装を提供するものであり、アカウント、課金、サービス停止、データ、コンプライアンス上のリスクは利用者が負います。
- 本番環境では HTTPS、強いランダムシークレット、管理画面のアクセス制限を設定してください。

## 主な機能

- 複数の上流アカウント管理。
- ユーザー向け API キー、上限、課金管理。
- OpenAI、Anthropic、Gemini などの互換 API 転送。
- アカウントスケジューリング、スティッキーセッション、同時実行制御、レート制限。
- リクエスト監査、エラー記録、管理画面での検索。
- 設定可能な上流エラー処理ルール。
- ユーザー、アカウント、モデル、課金、監視、システム設定の管理画面。
- 支払い機能の設定は [docs/PAYMENT.md](docs/PAYMENT.md) を参照してください。

## 技術スタック

| コンポーネント | 技術 |
| --- | --- |
| バックエンド | Go, Gin, Ent |
| フロントエンド | Vue 3, Vite, TailwindCSS |
| データベース | PostgreSQL |
| キャッシュ/キュー | Redis |
| デプロイ | Docker Compose, GitHub Actions, GHCR |

## Docker Compose デプロイ

`deploy/docker-compose.local.yml` の利用を推奨します。データがローカルディレクトリに保存されるため、バックアップと移行が容易です。

```bash
cd deploy
cp .env.example .env
```

`.env` で最低限以下を設定してください。

```bash
POSTGRES_PASSWORD=your_secure_password_here
JWT_SECRET=your_jwt_secret_here
TOTP_ENCRYPTION_KEY=your_totp_key_here
ADMIN_EMAIL=admin@example.com
ADMIN_PASSWORD=your_admin_password
SERVER_PORT=8080
```

ランダムシークレットの生成例：

```bash
openssl rand -hex 32
```

サービス起動：

```bash
docker compose -f docker-compose.local.yml up -d
```

状態とログの確認：

```bash
docker compose -f docker-compose.local.yml ps
docker compose -f docker-compose.local.yml logs -f sub2api
```

アップグレード：

```bash
docker compose -f docker-compose.local.yml pull
docker compose -f docker-compose.local.yml up -d
```

## アクセスと初期化

起動後、次の URL にアクセスします。

```text
http://YOUR_SERVER_IP:8080
```

初回起動時はセットアップ画面に従い、データベース、Redis、管理者アカウントを設定してください。

## Nginx リバースプロキシ

Nginx でリバースプロキシし、下線を含むリクエストヘッダーを使うクライアントに対応する場合、Nginx の `http` ブロックに以下を追加してください。

```nginx
underscores_in_headers on;
```

## 設定の要点

主な設定例は `deploy/config.example.yaml`、Docker 用の環境変数例は `deploy/.env.example` にあります。

本番環境では以下を確認してください。

- `jwt.secret` または `JWT_SECRET` に強いランダム値を設定する。
- `totp_encryption_key` または `TOTP_ENCRYPTION_KEY` に強いランダム値を設定する。
- `server.trusted_proxies` を信頼できるリバースプロキシに限定する。
- `cors.allowed_origins` を実際のフロントエンドドメインに限定する。
- `security.url_allowlist` を実際に必要な上流ドメインに限定する。
- 管理画面は信頼できるネットワークまたは認証入口からのみ公開する。
- PostgreSQL と Redis のデータをバックアップ対象にする。

## シンプルモード

個人または内部チーム向けに、一部の SaaS と課金関連機能を隠すモードです。

```bash
RUN_MODE=simple
SIMPLE_MODE_CONFIRM=true
```

## ソース開発

バックエンド：

```bash
cd backend
go run ./cmd/server
```

フロントエンド：

```bash
cd frontend
pnpm install
pnpm run dev
```

Ent schema を変更した場合：

```bash
cd backend
go generate ./ent
go generate ./cmd/server
```

## ディレクトリ構成

```text
sub2api-request-audit/
├── backend/                 # Go バックエンド
├── frontend/                # Vue フロントエンド
├── deploy/                  # Docker Compose とデプロイ設定
├── docs/                    # ドキュメント
└── .github/workflows/       # GitHub Actions
```

## 保守方針

このリポジトリでは上流変更を確認し、必要な bugfix、セキュリティ修正、互換性修正のみを選択して取り込みます。不要な機能変更やプロモーション内容は自動的に同期しません。

## License

This project is licensed under the [GNU Lesser General Public License v3.0](LICENSE) or later.
