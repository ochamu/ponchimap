# ponchimap の詳しい資料

[README](../README.md) に載せていない仕様をまとめます。コマンド一覧は `ponchimap help`、各コマンドの引数は `-h` で確認できます。

## 初期化

`init` は `.ponchimap/findings/`、表示設定 `architecture.view.yaml`、Claude Code と Codex の Skill・参照ファイルを作成します。CLI は Git 管理外でも使えますが、エージェントに調査を頼むフォルダでは先に `git init` を実行します。

`.gitignore` に作業領域全体の指定がなければ `/.ponchimap/*` を追加します。既存の `.gitignore` が読み取り専用などで使えないと `init.gitignore_unusable`（`partial`）、除外対象が追跡済みなら `init.tracked` で警告します。

再実行では不足分だけを追加し、同梱版と異なる Skill・参照ファイルは保持して `init.skill_differs` で知らせます。同梱版に替えるには `.claude/skills/ponchimap` と `.agents/skills/ponchimap` を退避して `init` を再実行します。

## 調査入力

1回の調査を1つの JSON ファイルに記録します。`nodes` は構成要素、`edges` は関係、`evidence` は各要素の根拠、`coverage` は調査範囲と結果です。書き方は [skills/ponchimap/references/findings.md](../skills/ponchimap/references/findings.md)（英語）、スキーマは [schema/findings.schema.json](../schema/findings.schema.json) にあります。

`import` は `.ponchimap/findings/` 直下の `*.json` を読みます（別の場所は `--input` で指定）。モデルを最初から作り直すには `.ponchimap/model.json` と `.ponchimap/acquisitions.json` を消して再度 `import` します。

## 利用者補足

`.ponchimap/supplements.yaml` を置くと、`import` が既存要素への利用者の説明（`stated`）として取り込みます。新しい要素は調査入力に書きます。記入例は [examples/first-run/supplements.yaml](../examples/first-run/supplements.yaml)、スキーマは [schema/supplements.schema.json](../schema/supplements.schema.json) にあります。

`target_id` はモデルのノードか関係の ID、`supports` の `connection.target` はモデルのノード ID です。対象がない項目や接続先がノードでない項目は `import.unknown_target` で警告し、取り込みません。

ノードごとに、`valid_time` のない補足の `role` が1種類なら `attributes.role` に優先して採用し、図とレポートに使います。補足ファイルがある回は前回分を置き換え、ない回は対象と参照先が残る補足を引き継ぎます。

## 表示設定

`architecture.view.yaml` で題、表示名、境界の表示名、グループ、集約、非表示、配置を指定します（記入例は `init` が作る [ひな形](../internal/workspace/templates/architecture.view.yaml)、スキーマは [schema/view.schema.json](../schema/view.schema.json)）。要素の ID はレポートの見出しか `import --json` で分かります。

`boundaries` の `member` にはその境界内のモデルのノード ID（例 `n-1p6t4dhx`、手書きの `n101`）を書き、`level` には `owner`（既定。アカウントやプロジェクト）、`region`、`network` のどれかを指定します。レポートの境界 ID（`bd_o_…` など）は使えません。ほかのノードが増減しても表示名は残り、名指ししたノードが別の境界に移ると表示名も一緒に移ります。

## PNG の出力

`render` は SVG の縦横2倍で PNG を描き、失敗したら等倍を試します。

- 等倍で描ければ PNG を保存します（警告 `render.png_reduced`、JSON の `status` は `ok`）。
- 等倍でも描けなければ SVG とレポートだけを保存し、同じ出力先の古い PNG を削除します（警告 `render.png_missing`、`status` は `partial`、終了コード `0`）。

図に BIZ UDPGothic で描けない文字があると、`render.missing_glyph` が Unicode コードポイントを示し、保存前に終了コード `2` で止まります。

## 終了コードと JSON

| 終了コード | JSON の `status` | 意味 |
|---|---|---|
| `0` | `ok` | 完了。警告が出る場合もある |
| `0` | `partial` | 一部だけ完了 |
| `2` | `error` | 引数または入力の誤り |
| `4` | `error` | 保存や配置の失敗、実行の取り消し、本体の欠陥 |

`init`、`import`、`lint`、`render` は診断と結果を標準エラーに書きます。`--json` 指定時の標準出力は JSON だけです。ヘルプ本文（`help`、`-h`）は標準出力、`--json` 指定時は標準エラーです。`version` は標準出力に書き、`version --json` は終了コード `2` です。

- `diagnostics` の各項目は `severity`、`code`、`message`、`location` を持ち、`hint`、`target_id` は任意です。`location` の `pointer`（JSON Pointer）は必須で、`path`、`line`、`column` は判明時に付きます。
- `artifacts` は作成・保存したファイルやディレクトリの配列です。`kind` が `model` なら `ids` に候補 ID とモデル ID の対応、`png` なら `scale` に倍率が入ります。

## 診断

診断には重大度、固定のコード、英語の説明が並びます。先頭の `ファイル:行:桁`、末尾の JSON Pointer や対象 ID は判明したものだけ表示します。`hint:` は修正方法です。

| コード | 意味 |
|---|---|
| `usage.*` | 引数の誤り |
| `input.*`、`schema.*` | 読み込み・構文・入力上限の問題、スキーマ違反 |
| `import.*` | 取り込みのエラーや警告 |
| `model.*` | モデルのエラーや警告。根拠のない要素があると描画を中止 |
| `view.*`、`lint.*` | 表示設定や配置、境界のエラーや警告 |
| `unverified.*`、`mismatch.*` | 実環境と未照合、または比較できる根拠の不一致 |
| `coverage.*` | 完了しなかった調査。権限不足、失敗、未実施など |
| `render.*`、`output.*`、`run.*` | 描画、保存、取り消し。`output.not_ours` は既存の図を保護 |
| `init.*` | 初期化の警告 |
| `tool.defect` | 本体の欠陥。再現用の入力を添えて報告 |

## ライセンス

本体と同梱アイコンは [Apache License 2.0](../LICENSE)、SVG に埋め込む BIZ UDPGothic は [SIL Open Font License 1.1](../internal/assets/fonts/OFL.txt) です。

| CLI の依存モジュール・同梱物 | ライセンス |
|---|---|
| D2（`github.com/d2lang/d2`、TALA を含む） | Mozilla Public License 2.0 |
| ELK の Go 版（`github.com/d2lang/elk-go`） | Eclipse Public License 2.0 |
| D2 と Chroma のフォント（Source Sans Pro、Source Code Pro、Fuzzy Bubbles、Noto Color Emoji、Liberation Mono） | SIL Open Font License 1.1 |
| その他の依存モジュール | MIT、BSD、Apache License 2.0、ISC |
