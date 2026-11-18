# メモアプリ

ブラウザでメモを書いて保存し、メールで共有できるアプリです。画面は `web/`、API は `worker/` にあります。

## 開発

```bash
npm install
cp .dev.vars.example .dev.vars   # 値は担当者に聞く
npm run dev:api
npm run dev:web
```

## デプロイ

| 環境 | コマンド |
|---|---|
| 開発（dev） | `npm run deploy:dev` |
| 本番（production） | `npm run deploy:production` |

DB を変えるときは `migrations/` に SQL を加え、`npm run db:migrate:dev` か `npm run db:migrate:production` で当てます。

運用の決まりは [docs/operations.md](docs/operations.md) にあります。
