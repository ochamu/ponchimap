<div align="center">

# ponchimap

アプリが使うクラウドのサービスとその関係を、根拠の付いた構成図にするツールです。<br>
コーディングエージェントが同梱の Skill に従ってリポジトリと資料に加えて実環境を調べ、ponchimap の CLI が図とレポートを書きます。

<a href="docs/images/order-service.png"><img src="docs/images/order-service.png" alt="ponchimap が描いた構成図の例。Cloudflare Workers、Cloud Firestore、Stripe、Vercel などのカードが、アカウントとプロジェクトの枠の中と外で矢印につながっている" width="860"></a>

架空の注文受付サービスを描いた図

</div>

## 特徴

- 図の箱と線には、どれも根拠があります。設定ファイルの宣言、実装、資料の記述、エージェントがサービスの CLI で実環境を照会した結果のどれを根拠にしたかは、レポートで読めます。
- 設定ファイルに書いてあるだけで実環境と照合していない要素には「未照合」の印が、根拠どうしで値が食い違う要素には「不一致」の印が付きます。資料や手がかりにしか出てこないものは破線で描きます。
- 特定のクラウドに限りません。ponchimap の Skill と CLI はサービスごとの知識を持たず、設定ファイルの読み方や照会のコマンドは、エージェントが各サービスの CLI のヘルプと公式の資料で確かめます。

## はじめる

Go 1.27.4 以降が要ります。ponchimap の CLI を入れます。

```bash
go install github.com/ochamu/ponchimap/cmd/ponchimap@latest
```

`go install` は `ponchimap` を Go の bin フォルダ（通常 `~/go/bin`）に置きます。そのフォルダを PATH に入れておきます。エージェントも PATH から `ponchimap` を探します。

構成図を描きたいアプリのフォルダで `init` を実行します。

```bash
ponchimap init
```

そのフォルダで Claude Code や Codex などのエージェントを起動し、次のように頼みます。

```text
このリポジトリの構成図を作って
```

Skill を名指しして頼むこともできます。Claude Code では `/ponchimap`、Codex では `$ponchimap` を依頼の頭に付けます。

エージェントは `init` が置いた Skill に従って、設定ファイル、コード、資料を読みます。環境が複数あって依頼が名指していなければ、どの環境を描くかを尋ねます。サービスの CLI のサインイン先が対象のアカウントと合わないときも、照会の前に尋ねます。調べたことは調査入力（JSON のファイル）に書き、ponchimap の CLI で図とレポートにします。最後に、図とレポートの場所、主な要素の役割、確かめられなかった点とその理由を答えます。

もう一度頼むと、エージェントは前の調査入力を読んだうえでリポジトリと資料を調べ直し、同じ要素を同じIDのまま更新します。`architecture.view.yaml` に書いた表示名やグループは残ります。

### プラグインとして入れる

Skill は `init` で置く代わりに Claude Code や Codex のプラグインとして入れることもできます。一度入れれば、どのリポジトリでも `init` を自分で実行せずに頼めます。CLI は上の手順のとおりに入れておきます。

Claude Code では次のコマンドで入れます。

```bash
claude plugin install ponchimap --marketplace ochamu/ponchimap
```

Codex では次の2つのコマンドで入れます。

```bash
codex plugin marketplace add ochamu/ponchimap
```

```bash
codex plugin add ponchimap@ponchimap
```

入れたあとに起動したエージェントで、Claude Code では `/ponchimap:ponchimap`、Codex では `$ponchimap:ponchimap` を依頼の頭に付けて頼みます。プラグインの Skill はまず `ponchimap init` を実行し、リポジトリに置かれた Skill に従って続けます。初めて頼むリポジトリでは、入れてある CLI に同梱の Skill を `init` が置きます。違う Skill がすでに置いてあるときは、`init` はそれを残して警告し、エージェントがその警告を伝えます。

## 見本で試す

エージェントもクラウドのアカウントも使わずに、図ができるまでを試せます。手で書いた調査入力の見本を取り込んで描きます。見本には、設定ファイルを読んだ結果とクラウドに照会した結果を、架空の値で書いてあります。見本はこのリポジトリにあるので、まず `git clone` で取得します。ponchimap のコマンドはどれもネットワークを使いません。

```bash
git clone https://github.com/ochamu/ponchimap.git ~/ponchimap
mkdir -p ~/ponchimap-demo && cd ~/ponchimap-demo
ponchimap init
cp ~/ponchimap/examples/first-run/findings/*.json .ponchimap/findings/
ponchimap import && ponchimap lint && ponchimap render
```

`docs/architecture/overview.png` に、見本の図（[examples/first-run/overview.png](examples/first-run/overview.png)）と同じ図ができます。

エージェントで試すときは、見本のアプリ `~/ponchimap/examples/memo-app` を `~/ponchimap` の外に写して `git init` し、「はじめる」と同じように `ponchimap init` を実行してから頼みます。見本には dev と production の2つの環境があり、どちらを描いてもかまいません。識別子は架空の値なので、頼むときに「クラウドへの照会はしないで」と添えます。

## できるもの

| ファイル | 中身 |
|---|---|
| `docs/architecture/overview.svg`、`overview.png` | 構成図 |
| `.ponchimap/report.md` | 要素ごとの役割、根拠、確かめられなかった点のレポート |
| `architecture.view.yaml` | 表示設定。題、表示名、グループ、配置などの見せ方を書く |
| `.ponchimap/` | 調査入力（`findings/`）、モデル、取り込みの記録を置く作業領域 |

## 図を整える

見せ方だけを変えたいときは、エージェントに「この2つを『バックエンド』という枠でまとめて」のように頼むか、`architecture.view.yaml` を書き換えて `ponchimap render` を実行します。表示設定は見せ方だけを変え、根拠は変えません。

配置が読みにくいときは、配置エンジンと向きを替えるか、配置の seed を引き直して描き直します。配置エンジンを ELK に替えて上から下へ並べるには、次のコマンドを実行します。

```bash
ponchimap render --engine elk --direction down
```

既定の配置エンジン TALA で描いた図は、次のコマンドで seed を引き直せます。要素の少ない図は seed を変えても同じ図になることがあります。

```bash
ponchimap render --reroll
```

続けて使う配置は `architecture.view.yaml` の `layout` に書きます。

## 安全のために

- Skill はエージェントに、読み取りのコマンドだけを実行するように指示しています。実行の前に、コマンドが読み取りだけであることと、出力に資格情報や秘密の値が載らないことを、ヘルプか公式の資料で確かめさせます。デプロイ、作成、削除、サインイン、CLI のインストールはしないようにも書いています。
- 作業領域 `.ponchimap/` は、`init` が `.gitignore` に加える行で Git の対象から外れます。図、表示設定、Skill のフォルダは作業領域の外にあり、コミットするかどうかは利用者が選びます。
- レポートはアカウントIDなどの識別子の値を含みます。手元で読むための記録なので、そのまま共有しないでください。
- 図には、調査入力に書いた役割や関係の説明がそのまま出ます。識別子や秘密の値を書いていないかを、共有する前に図で確かめてください。

## 詳しい資料

- [docs/reference.md](docs/reference.md) は、利用者補足、表示設定、終了コード、診断の読み方をまとめています。
- [skills/ponchimap/SKILL.md](skills/ponchimap/SKILL.md) は、エージェントが従う手順です（英語）。
- [skills/ponchimap/references/findings.md](skills/ponchimap/references/findings.md) は、調査入力の書き方です（英語）。スキーマは [schema/](schema/) にあります。

## ライセンス

Apache License 2.0 です（[LICENSE](LICENSE)）。同梱のフォント BIZ UDPGothic は SIL Open Font License 1.1 です。依存するモジュールのライセンスは [docs/reference.md](docs/reference.md#ライセンス) にあります。
