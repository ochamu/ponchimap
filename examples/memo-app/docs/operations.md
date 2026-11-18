# 運用メモ

## バックアップ

本番のメモ（D1 の `memo-db`）は、毎晩3時（日本時間）に書き出して、同じ Cloudflare アカウントの R2 バケット `memo-backups` に置いている。書き出すジョブは運用用の `memo-ops` リポジトリの GitHub Actions にあり、このリポジトリにはない。バケットはダッシュボードで作ったもので、このリポジトリの設定には出てこない。

開発環境の `memo-db-dev` はバックアップしていない。

## 復元

`memo-backups` から書き出したファイルを取り出し、`wrangler d1 execute memo-db --remote --env production --file <ファイル>` で流し込む。流し込む前に、共有メールの機能を止めることを利用者に知らせる。
