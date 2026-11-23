<div align="center">

# ponchimap

アプリが使うクラウドのサービスとその関係を、根拠付きの構成図にするツールです。<br>
コーディングエージェントが同梱の Skill に従ってリポジトリ・資料・実環境を調べ、ponchimap の CLI が図とレポートを生成します。

<a href="docs/images/order-service.png"><img src="docs/images/order-service.png" alt="ponchimap が描いた構成図の例。Cloudflare Workers、Cloud Firestore、Stripe、Vercel などのカードが、アカウントとプロジェクトの枠の中と外で矢印につながっている" width="860"></a>

*架空の注文受付サービスを描いた構成図の例*

</div>

## 特徴

エージェントに構成図を直接描かせると、推測と確かめた事実が混ざり、実環境とのずれが見えず、描き直すたびに図が変わります。ponchimap は調べる作業だけをエージェントに任せ、調査入力をモデルにまとめる、検査する、描くの3つは、ネットワークを使わない Go の CLI が受け持ちます。同じ状態のモデルに同じ調査入力と同じ設定を与えれば、同じ図とレポートが出ます。

- **すべての要素に根拠が残る**: 図の箱と線には、設定ファイルの宣言・実装・資料の記述・サービスの CLI による実環境照会のどれを根拠にしたかが記録され、レポートで確認できます。
- **確かさの度合いが図で分かる**: 根拠の状態に応じて図の表現が変わります。
  - **「未照合」の印**: 設定ファイルに書いてあるだけで、実環境と照合していない要素
  - **「不一致」の印**: 複数の根拠どうしで値が食い違っている要素
  - **破線**: 資料や手がかりにしか出てこない要素
- **特定のクラウドに依存しない**: ponchimap の Skill と CLI はサービス固有の知識を持ちません。設定ファイルの読み方や照会コマンドは、エージェントが各サービスの CLI ヘルプと公式資料で確かめます。

### 不一致を図とレポートで見る

見本 [testdata/samples/long-labels-and-mismatches](testdata/samples/long-labels-and-mismatches/) では、Cloudflare Workers の設定項目 `DB_PRIMARY` の接続先が、`wrangler.toml` の宣言と実環境の照会結果で違います。図ではそのカードに「不一致」の印が付き、2本の線のラベルの末尾に「（不一致）」が付きます。資料にしか出てこない OpenAI API は破線で、実環境と照合していない Cloudflare R2 には「未照合」の印が付きます。

<a href="docs/images/long-labels-and-mismatches.png"><img src="docs/images/long-labels-and-mismatches.png" alt="不一致と未照合を含む構成図の例。Cloudflare Workers と Cloud Functions for Firebase のカードに「不一致」の印、Cloudflare R2 のカードに「未照合」の印が付き、OpenAI API が破線で描かれている" width="860"></a>

<details>
<summary>同じ見本から生成したレポートの「食い違い」の抜粋（リンクとエスケープの記号は省略）</summary>

```markdown
- 不一致: n301 Cloudflare Workers、e301 接続設定（n301 → n302）、e302 接続設定（n301 → n303）。設定項目（slot）「DB_PRIMARY」の接続先が根拠によって違う
  - ev315: e301 接続設定（n301 → n302）の設定上の宣言（declared）。`connection.configured = true`、`connection.target = "n302"`。出典 file `wrangler.toml` 10〜16行。確認時刻 2026-11-13T09:05:00Z。対象時点 2026-11-13T09:00:00Z
  - ev316: e302 接続設定（n301 → n303）の実環境で確認（deployed）。`connection.configured = true`、`connection.target = "n303"`。出典 query `findings/long-labels-and-mismatches/primary-binding.json`。確認時刻 2026-11-13T09:05:00Z。対象時点 2026-11-13T09:00:00Z
  - この項目に当たる調査範囲: cv303 完了（complete）。DB_PRIMARYの単一接続先だけを対象時点で取得完了。実環境の接続先はn303
```

</details>

## しくみ

```mermaid
flowchart LR
  subgraph agent["調べる側: コーディングエージェント（Skill に従う）"]
    sources["リポジトリ、資料、各サービスの CLI"]
  end
  subgraph tool["描く側: ponchimap CLI（ネットワークなし）"]
    imp["import"] --> model[".ponchimap/model.json（事実と根拠）"]
    model --> lint["lint"]
    model --> render["render"]
  end
  sources -- "調査入力 JSON（schema/ で検査）" --> imp
  viewcfg["architecture.view.yaml（見せ方だけ）"] --> lint
  viewcfg --> render
  render --> out["図とレポート"]
```

- 調べる側と描く側の境は [schema/](schema/) の JSON Schema です。CLI は調査入力をスキーマで検査して取り込むだけで、リポジトリや実環境を読み直しません。サービスの知識はバージョンごとに変わり、CLI に書くと古くなるからです。
- `.ponchimap/model.json` が事実と根拠を持ち、`architecture.view.yaml` は見せ方だけを変えます。要素を隠してもまとめても、未照合と不一致の判定は変わりません。
- モデルと表示設定の検査は `lint` の1か所にあり、`render` はその検査を通ったものだけを描きます。`import` も保存の前に同じ検査を通し、エラーのあるモデルは保存しません。
- 同じ要素は再調査でも同じ ID を保ち、表示設定が要素から外れません。ID は主キーのハッシュから作るので、どの作業領域でも同じ主キーからは同じ ID が出ます。
- 描画エンジンの D2 を使うのは `internal/render/d2render` だけです。ほかのパッケージが D2 を import するとテストが失敗します。
- 取り込みの規則は [internal/importer/doc.go](internal/importer/doc.go)、根拠の評価の規則は [internal/view/assess.go](internal/view/assess.go) の doc コメントが本文です。`testdata/` の期待値はその規則から手で導いています。

## はじめる

### 1. CLI をインストールする

Go 1.27.4 以降が必要です。

```bash
go install github.com/ochamu/ponchimap/cmd/ponchimap@latest
```

`go install` は `ponchimap` を Go の bin フォルダ（通常 `~/go/bin`）に配置します。エージェントも PATH から `ponchimap` を探すため、このフォルダを PATH に通しておいてください。

### 2. 構成図を描きたいフォルダで初期化する

対象のアプリのフォルダで `ponchimap init` を実行します。

```bash
ponchimap init
```

> [!TIP]
> 複数のリポジトリで使う場合は、[プラグインとして入れる](#プラグインとして入れる)と事前の `ponchimap init` を省略できます。

### 3. エージェントに依頼する

そのフォルダで Claude Code や Codex などのエージェントを起動し、次のように頼みます。

```text
このリポジトリの構成図を作って
```

Skill を明示して頼む場合は、依頼の先頭に **Claude Code では `/ponchimap`**、**Codex では `$ponchimap`** を付けます。

#### 実行の流れ

1. **調査**: `init` が配置した Skill に従い、設定ファイル・コード・資料を読み、必要に応じてサービスの CLI で実環境を照会します。
   - 環境が複数あって依頼で指定されていない場合は、どの環境を描くかを尋ねます。
   - サービスの CLI のサインイン先が対象アカウントと合わないときも、照会の前に尋ねます。
2. **出力**: 調べた結果を調査入力（JSON）に書き出し、ponchimap の CLI で構成図とレポートを生成します。
3. **報告**: 図とレポートの場所、主な要素の役割、確かめられなかった点とその理由を答えます。

**構成図を更新したいとき**: もう一度同じように頼むと、エージェントは前回の調査入力を読んだうえでリポジトリと資料を調べ直し、同じ要素を同じ ID のまま更新します。`architecture.view.yaml` に書いた表示名やグループもそのまま残ります。

### プラグインとして入れる

プラグインを一度入れておくと、まだ `ponchimap init` を実行していないアプリのフォルダでも、Claude Code や Codex に直接構成図を頼めます（CLI のインストールは事前に必要です）。

#### インストール

**Claude Code**

```bash
claude plugin install ponchimap --marketplace ochamu/ponchimap
```

**Codex**

```bash
codex plugin marketplace add ochamu/ponchimap
codex plugin add ponchimap@ponchimap
```

#### 使い方

インストール後にエージェントを起動し、依頼の先頭に **Claude Code では `/ponchimap:ponchimap`**、**Codex では `$ponchimap:ponchimap`** を付けて頼みます。

エージェントは指定したアプリのフォルダ（未指定なら現在の作業フォルダ）で `ponchimap init` を実行し、そのまま調査と描画を続けます。

> [!NOTE]
> **既存の Skill がある場合**: `init` は同梱の Skill と参照ファイルのうちフォルダにないものだけを追加します。既存ファイルの内容が同梱版と異なる場合は上書きせず残して警告し、エージェントはその警告を伝えたうえでフォルダ内の手順と参照ファイルに従います。

## 見本で試す

リポジトリに同梱されている見本を使って、動作を手元で試せます。

### CLI だけで試す（エージェント・クラウド不要）

手で書いた調査入力の見本（設定ファイルの読み取り結果とクラウド照会結果を架空の値で記述したもの）を取り込んで描画します。ponchimap のコマンドはどれもネットワークを使いません。

```bash
git clone https://github.com/ochamu/ponchimap.git ~/ponchimap
mkdir -p ~/ponchimap-demo && cd ~/ponchimap-demo
ponchimap init
cp ~/ponchimap/examples/first-run/findings/*.json .ponchimap/findings/
ponchimap import && ponchimap lint && ponchimap render
```

実行後、`docs/architecture/overview.png` に [examples/first-run/overview.png](examples/first-run/overview.png) と同じ図が生成されます。

### エージェントで試す（見本アプリ）

見本アプリ `examples/memo-app` を使って、エージェントの調査から描画までを試せます（先に上の `git clone` で `~/ponchimap` を取得しておきます）。

1. 見本アプリを `~/ponchimap` の外にコピーし、`git init` と `ponchimap init` を実行します。

   ```bash
   cp -r ~/ponchimap/examples/memo-app ~/memo-app-demo
   cd ~/memo-app-demo
   git init
   ponchimap init
   ```

2. エージェントを起動し、**「クラウドへの照会はしないで」** と添えて構成図の作成を頼みます（見本アプリの識別子は架空の値のため）。
   - 見本には `dev` と `production` の 2 つの環境があり、どちらを描いてもかまいません。

## できるもの

| ファイル | 中身 |
|---|---|
| `docs/architecture/overview.svg`<br>`docs/architecture/overview.png` | 構成図（SVG / PNG）。PNG は描けた回だけ保存します。描けなかった回は SVG とレポートだけを保存し、古い PNG を消して `partial` で終わります |
| `.ponchimap/report.md` | 要素ごとの役割、根拠、確かめられなかった点のレポート |
| `architecture.view.yaml` | 表示設定（題、表示名、グループ、配置などの見せ方） |
| `.ponchimap/` | 作業領域（調査入力 `findings/`、モデル、取り込みの記録） |

> [!NOTE]
> **出力の言語**: 図の凡例と印、レポートの定型文は日本語です。同梱フォント（BIZ UDPGothic）は日本語とラテン文字を持ち、フォントにない文字がラベルにあると `render` はその文字を名指して止まります。CLI の診断は英語です。エージェントは利用者の言語で答えます。

## 図を整える

### 見せ方を変える

エージェントに「この2つを『バックエンド』という枠でまとめて」のように頼むか、`architecture.view.yaml` を編集して `ponchimap render` を実行します。表示設定は見た目だけを変え、根拠は変えません。

### 配置を調整する

配置が読みにくいときは、配置エンジンや向き、seed を変えて描き直せます。気に入った配置を続けて使うには、`architecture.view.yaml` の `layout` に書きます。

配置エンジンを ELK に替え、上から下へ並べるには次のコマンドを実行します。

```bash
ponchimap render --engine elk --direction down
```

既定の配置エンジン（TALA）で配置の seed を引き直すには次のコマンドを実行します（要素の少ない図は seed を変えても同じ図になることがあります）。

```bash
ponchimap render --reroll
```

## 安全のために

- **読み取り専用での調査**: Skill はエージェントに対し、読み取りのコマンドだけを実行するよう指示しています。実行前には、コマンドが読み取り専用であることと、出力に資格情報や秘密の値が載らないことを、ヘルプか公式資料で確かめさせます。デプロイ・作成・削除・サインイン・CLI のインストールはしない指示になっています。
- **CLI はネットワークを使わない**: `init`、`import`、`lint`、`render` はどれも通信しません。フォント、アイコン、スキーマは同梱で、PNG もプロセス内で描きます。実環境を照会するのはエージェントだけで、その照会は Skill の読み取り専用の決まりに従います。
- **作業領域は Git 管理から除外**: `.ponchimap/` は、`init` が `.gitignore` に加える行で Git の追跡対象から外れます。作業領域を名指す行がすでにあれば足さず、すでに Git が追跡しているファイルは消さずに警告します。図・表示設定・Skill のフォルダは作業領域の外にあり、コミットするかどうかは利用者が選べます。

> [!WARNING]
> **共有する前に確認してください**
> - **レポートはそのまま共有しない**: `.ponchimap/report.md` はアカウント ID などの識別子の値を含みます。手元で読むための記録として扱ってください。
> - **図の内容を確認してから共有する**: 構成図には、調査入力に書いた役割や関係の説明がそのまま出ます。識別子や秘密の値を書いていないかを、共有する前に図で確かめてください。

## 詳しい資料

- [docs/reference.md](docs/reference.md) — 利用者補足、表示設定、終了コード、診断の読み方
- [SECURITY.md](SECURITY.md) — 脆弱性の報告先
- [skills/ponchimap/SKILL.md](skills/ponchimap/SKILL.md) — エージェントが従う手順（英語）
- [skills/ponchimap/references/findings.md](skills/ponchimap/references/findings.md) — 調査入力の書き方（英語、スキーマは [schema/](schema/)）

## ライセンス

- **ponchimap 本体**: [Apache License 2.0](LICENSE)
- **同梱フォント（BIZ UDPGothic）**: SIL Open Font License 1.1
- **依存モジュール**: [docs/reference.md](docs/reference.md#ライセンス) を参照
