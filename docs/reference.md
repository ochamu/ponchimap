# ponchimap の詳しい資料

[README](../README.md) に載せていない決まりをまとめます。引数の一覧は `ponchimap help` と各コマンドの `-h` で読めます。

## init が用意するもの

`init` が用意するのは、作業領域 `.ponchimap/`、調査入力を置く `.ponchimap/findings/`、表示設定のひな形 `architecture.view.yaml`、作業領域を Git の対象から外す `.gitignore` の行と、エージェントが読む Skill です。もう一度実行しても、すでにあるものは変えません。置いてある Skill が、実行した CLI に同梱の Skill と違うときは、警告で知らせます。新しいバージョンの Skill に替えるには、`.claude/skills/ponchimap` と `.agents/skills/ponchimap` を消してから `init` を実行します。Git がすでに追跡している作業領域のファイルのうち、除外の行に当たるものは消さずに警告で知らせます。

Git のリポジトリでないフォルダでも動きます。ただし Skill は、Git が無視するファイルを開かないために Git のリポジトリの中でだけリポジトリを読みます。エージェントに頼むフォルダでは、先に `git init` を実行しておきます。

## 調査入力を手で書く

調査入力は構成について調べたことを書いた JSON のファイルです。1つのファイルが1回の調査を表し、構成要素（`nodes`）、関係（`edges`）、それぞれの根拠（`evidence`）を持ちます。書き方は Skill の [skills/ponchimap/references/findings.md](../skills/ponchimap/references/findings.md)（英語）に、スキーマは [schema/findings.schema.json](../schema/findings.schema.json) にあります。

最初からやり直すには、`.ponchimap/model.json` と `.ponchimap/acquisitions.json` を消してから `import` を実行します。

## 利用者補足

利用者補足は、モデルにある要素について利用者が知っていることを書き足す YAML のファイルです。`.ponchimap/supplements.yaml` に置くと `import` が読み、書いた内容を資料や利用者の説明（`stated`）の根拠として取り込みます。役割（`role`）を書くと、図とレポートの役割がその文になります。要素は加えられないので、新しい要素は調査入力に書きます。

見本の [examples/first-run/supplements.yaml](../examples/first-run/supplements.yaml) は、見本の API のノードに役割を書き足します。

```yaml
schema_version: "0.1"
statements:
  - target_id: n-1p6t4dhx
    supports:
      - key: role
        value: メモの読み書きを受け付けるAPI
    source: 利用者の説明
    checked_at: "2026-11-17T10:00:00Z"
```

`supports` は調査入力の根拠と同じ形です。モデルにないIDを書いた項目は、警告（`import.unknown_target`）を出して取り込みません。スキーマは [schema/supplements.schema.json](../schema/supplements.schema.json) にあります。

## 表示設定

`architecture.view.yaml` には図の題、要素の表示名、境界の表示名、論理グループ、集約、非表示にする要素を書けます。表示設定は図の見せ方だけを変え、モデルの事実と根拠を変えません。書き方の例は `init` が作るひな形のコメントに、スキーマは [schema/view.schema.json](../schema/view.schema.json) にあります。

要素のIDはレポートの見出し（`### n-xxxxxxxx サービス名`）と、`import --json` の `artifacts` の `ids` で分かります。

境界（アカウントやプロジェクト、リージョン、ネットワーク）の表示名は、その境界に入っているノードを1つ名指しして付けます。

```yaml
boundaries:
  - member: n-1p6t4dhx   # その境界に入っているノードのID
    level: owner         # owner、region、network のどれか
    label: "メモアプリのアカウント"
```

`member` にはノードのID（`n-` で始まるID）を書きます。レポートの境界の見出しにあるID（`bd_o_…`）は書けません。名指ししたノードがその境界に入っている間は、ほかのノードが増減しても名前が残ります。ノードが別の境界に移ると、名前も一緒に移ります。

## 終了コード

| 終了コード | 意味 |
|---|---|
| 0 | 成功した。警告だけの場合と、一部だけできた場合（`partial`）を含む |
| 2 | 引数か入力を直せば通る失敗 |
| 4 | 実行の失敗。保存できない、配置できない、取り消された、本体の欠陥 |

`init`、`import`、`lint`、`render` の人が読む行は標準エラーに出ます。`help`、`version`、`-h` の文は標準出力に出ます。`init`、`import`、`lint`、`render` は、`--json` を付けると結果（`status`、`diagnostics`、`artifacts`）を JSON で標準出力に書きます。

- `status` は `ok`、`partial`、`error` のどれかです
- `diagnostics` の各項目は `severity`、`code`、`message`、`location` を持ちます。`location` は `pointer` を必ず持ち、`path`、`line`、`column` は分かるときだけ持ちます。`hint` と `target_id` も値があるときだけ入ります
- `artifacts` はコマンドが作ったものと保存したものの一覧です。`import` の `model` の項目は、調査入力の候補とモデルのIDの対応（`ids`）も持ちます

## 診断の読み方

診断は次の形で出ます。説明の文は英語で、コードは変わらない識別子です。

```text
ファイル:行:桁: 重大度 コード: 説明 (ファイルの中の場所, 対象のID)
  hint: 直し方
```

ファイル、行と桁、括弧の中は、分かるときだけ出ます。どの診断にも説明の文があり、多くは `hint:` の行に直し方が出ます。コードの最初の語で、どの種類の問題かが分かります。

| コード | 意味 |
|---|---|
| `usage.*` | 引数の誤り |
| `input.*` | ファイルを読めない、構文の誤り、対応しない書き方、上限を超える入れ子や数値 |
| `schema.*` | 値がスキーマに合わない |
| `import.*` | 調査入力を取り込めない（エラー）か、取り込んだ結果に確かめる点がある（警告） |
| `model.*` | モデルの誤りと警告。根拠のない要素があると図を描かない |
| `view.*`、`lint.*` | 表示設定と配置の値の誤り、表示設定と境界についての警告 |
| `unverified.*` | 実環境と照合できていない要素と関係。図に「未照合」の印が付く |
| `mismatch.*` | 比べられる根拠の値が違う要素。図に「不一致」の印が付く |
| `coverage.*` | 完了しなかった照会（権限不足、失敗、照会しなかった、など） |
| `render.*`、`output.*`、`run.*` | 描けない文字、配置の失敗、PNG の縮小、保存の失敗、出力先にある ponchimap が書いていない図、取り消し |
| `init.*` | `init` の警告 |
| `tool.defect` | 本体の欠陥。原因になった入力を添えて報告してください |

## ライセンス

ponchimap は Apache License 2.0 です（[LICENSE](../LICENSE)）。同梱のフォント BIZ UDPGothic は SIL Open Font License 1.1 です（[internal/assets/fonts/OFL.txt](../internal/assets/fonts/OFL.txt)）。図に埋め込まれるアイコンはこのリポジトリの著作物で、Apache License 2.0 です。図の文字には同梱のフォント BIZ UDPGothic を埋め込みます。

ビルドした CLI には依存するモジュールも入ります。主なものは次のとおりで、ほかは MIT、BSD、Apache License 2.0、ISC のモジュールです。

| モジュール | ライセンス |
|---|---|
| D2（`github.com/d2lang/d2`。配置エンジン TALA を含む） | Mozilla Public License 2.0 |
| ELK の Go 版（`github.com/d2lang/elk-go`） | Eclipse Public License 2.0 |
| D2 と chroma が持つフォント（Source Sans Pro、Source Code Pro、Fuzzy Bubbles、Noto Color Emoji、Liberation Mono） | SIL Open Font License 1.1 |
