package report

import (
	"fmt"
	"strings"

	"github.com/ochamu/ponchimap/internal/model"
	"github.com/ochamu/ponchimap/internal/view"
)

const (
	reportTitle = "構成の根拠レポート"

	preambleText = "このレポートは手元で読むための記録である。アカウントIDなどの識別子の値を載せているので、そのまま共有しない。"

	readingText       = "「未照合」は宣言を実環境と照合できていないことを表す。「不一致」は比べられる根拠の値が違うことを表す。どちらも書かれていない要素は、照合が要らない要素か、照合した項目が一致した要素である。書かれていないことは、すべての項目や現在の稼働を確かめたという意味ではない。"
	evidenceKindsText = "設定上の宣言（declared）は設定ファイルなどにある宣言、実環境で確認（deployed）は実環境から取得したその項目の値、呼び出しの実装（code）は呼び出し元と接続先を特定できる実装、通信の観測（observed）は対象期間の通信の記録、資料や利用者の説明（stated）は直接には確かめていない説明、手がかり（hint）はSDKや環境変数名などである。"
	validTimeText     = "対象時点のない根拠は、モデルが表す現在の状態を述べる。現在の状態の根拠と、対象時点のある根拠は比べていない。"

	mismatchLeadText  = "比べられる根拠の値が違う項目である。どちらの値が正しいかは判断していない。"
	noMismatchText    = "比べられる根拠の値が違う項目は見つかっていない。未確認の点に挙げた項目は、比べる相手がないので、ここには出ない。"
	noUnconfirmedText = "未照合の項目、根拠が手がかりや説明だけの要素、完了していない調査範囲は、どれもない。"
	noOmittedText     = "モデルの構成要素と関係は、どれも1枚のカードか1本の線として図に載っている。"
	coverageLeadText  = "完了（complete）は記録した範囲の取得が終わったことを表す。範囲の外の要素や、絞り込みにない項目には当てはまらない。完了していない範囲で見つからなかったものは、ないとは言えない。"
	noCoverageText    = "調査範囲の記録はない。"
	targetScopeLabel  = "調査の対象として記録した範囲"
	redrawText        = "フォントとアイコンはツールに含まれている。描き直しに LLM とクラウドへの照会は要らない。"

	noStateText           = "未照合と不一致はない"
	detailLabel           = "詳細"
	weakStateText         = "直接の根拠がない。手がかりや説明だけが支えている"
	weakLabel             = "手がかり・説明のみ"
	weakEntryText         = "直接の根拠がない"
	logicalMembershipText = "表示設定による論理グループで、所属の根拠は持たない"
	noRoleText            = "モデルに記載なし"
	undefinedKeyText      = "このツールが意味を定めていないキーなので、値は載せていない。値はモデルにある"
	noReasonText          = "理由の記録なし"
	noCoverageOfUnitText  = "この項目に当たる調査範囲の記録はない"
	settleDirectlyText    = "設定、実装、実環境、観測のいずれかで確かめ、その根拠を取り込む"
)

var nodeKindNames = map[model.NodeKind]string{
	model.KindResource: "管理対象のリソース",
	model.KindService:  "サービス一般",
}

var relationNames = map[model.Relation]string{
	model.RelConfiguration:  "接続設定",
	model.RelImplementation: "呼び出し実装",
	model.RelObservation:    "通信観測",
	model.RelDependency:     "起動・デプロイ依存",
}

var evidenceKindNames = map[model.EvidenceKind]string{
	model.EvDeclared: "設定上の宣言",
	model.EvDeployed: "実環境で確認",
	model.EvCode:     "呼び出しの実装",
	model.EvObserved: "通信の観測",
	model.EvStated:   "資料や利用者の説明",
	model.EvHint:     "手がかり",
}

var frameNames = map[view.Frame]string{view.FrameSolid: "実線の枠", view.FrameDashed: "破線の枠"}

var lineNames = map[view.Line]string{view.LineSolid: "実線", view.LineDashed: "破線"}

var groupKindNames = map[view.GroupKind]string{
	view.GroupBoundary: "実在する境界（所属の根拠から導出）",
	view.GroupLogical:  "説明のための論理グループ（表示設定で追加。所属の根拠は持たない）",
}

// An identifier the table does not know is written as it is.
func named[K ~string](names map[K]string, key K) string {
	if name, ok := names[key]; ok {
		return fmt.Sprintf("%s（%s）", name, string(key))
	}
	return string(key)
}

// unitMark stands in findingTexts for the name of what was checked.
const unitMark = "{項目}"

// Only an unverified state has a way to settle it.
var findingTexts = map[view.FindingCode]struct{ state, settle string }{
	view.UnverifiedNotChecked: {
		unitMark + "を実環境と照合していない",
		"この項目を実環境で照会し、結果を実環境の根拠（deployed）として取り込む",
	},
	view.UnverifiedSuperseded: {
		unitMark + "は以前に実環境で確かめたが、そのあとの照会が終わらなかったので、照合済みとしていない",
		"終わらなかった照会をやり直し、結果を取り込む",
	},
	view.UnverifiedAbsenceUnbacked: {
		unitMark + "が「ない」という取得結果はあるが、完了した調査範囲と結び付いていないので、照合に使っていない",
		"「ない」という結果を得た取得を、同じ取得記録を持つ完了（complete）の調査範囲として記録する",
	},
	view.UnverifiedStatedOnly: {"関係を支える根拠が資料や利用者の説明だけである", settleDirectlyText},
	view.MismatchValues:       {state: unitMark + "が根拠によって違う"},
	view.MismatchAbsent:       {state: unitMark + "について、宣言と、完了した取得の「ない」という結果が食い違う"},
}

var keyNames = map[string]string{
	model.KeyExistence:  "存在",
	model.KeyRegion:     "リージョン",
	model.KeyScopeOwner: "所属",
	model.KeyExposure:   "公開範囲",
}

func unitName(f view.Finding) string {
	switch {
	case f.Key != model.KeyConnectionTarget:
		return named(keyNames, f.Key)
	case f.Slot != "":
		return fmt.Sprintf("設定項目（slot）「%s」の接続先", f.Slot)
	}
	return "接続先"
}

// sentence says what a finding found, without the state it belongs to.
func sentence(f view.Finding) string {
	t, ok := findingTexts[f.Code]
	if !ok {
		return string(f.Code)
	}
	return strings.ReplaceAll(t.state, unitMark, unitName(f))
}

// citedKeys are the support keys whose values a finding prints. A connection
// is read from two keys, and the report prints both as they are written
// instead of deriving the target a second time.
func citedKeys(f view.Finding) []string {
	if f.Key == model.KeyConnectionTarget {
		return []string{model.KeyConnectionTarget, model.KeyConnectionConfigure}
	}
	return []string{f.Key}
}

var coverageTexts = map[model.CoverageStatus]struct{ name, settle string }{
	model.CovComplete:     {name: "完了"},
	model.CovPartial:      {"部分取得", "残りを取得し、結果を取り込む"},
	model.CovDenied:       {"権限不足", "照会に必要な権限を確かめ、照会をやり直す"},
	model.CovUnsupported:  {"未対応", "別の手段で確かめ、結果を取り込む"},
	model.CovNotAttempted: {"未実施", "照会を実施し、結果を取り込む"},
	model.CovFailed:       {"取得失敗", "照会をやり直し、結果を取り込む"},
}

func statusName(status model.CoverageStatus) string {
	if t, ok := coverageTexts[status]; ok {
		return fmt.Sprintf("%s（%s）", t.name, string(status))
	}
	return string(status)
}

// The marks of shownTexts stand for what the scene element that shows the
// element looks like, and for the hidden ends of an edge. figure.describe
// fills them in.
const (
	aggregateMark  = "{集約}"
	cardMark       = "{カード}"
	frameMark      = "{枠と印}"
	lineMark       = "{線}"
	hiddenEndsMark = "{非表示の端}"
)

// shownTexts gives, for each way a figure shows an element, the words of its
// 図での表示 row. An element without a shape of its own is also listed under
// 図に載せなかったもの: omittedAs is the label of its row there, and omitted
// what the row says when that differs from drawn.
var shownTexts = map[view.Shown]struct{ drawn, omittedAs, omitted string }{
	view.ShownCard:       {drawn: cardMark},
	view.ShownAggregated: {aggregateMark + "にまとめて表示。集約は" + frameMark, "集約", aggregateMark + "にまとめた"},
	view.ShownLine:       {drawn: lineMark},
	view.ShownFolded:     {drawn: "線を描いていない。両端が" + aggregateMark + "の内側にある", omittedAs: "集約の内側"},
	view.ShownSelf:       {drawn: "線を描いていない。接続元と接続先が同じである。状態はノードの印に含めている", omittedAs: "接続元と接続先が同じ"},
	view.ShownHidden:     {drawn: "図に載せていない。表示設定で非表示にした", omittedAs: "非表示"},
	view.ShownEndHidden:  {drawn: "線を描いていない。端の " + hiddenEndsMark + " を表示設定で非表示にした", omittedAs: "端が非表示"},
}

const sharedLineText = " と同じ線にまとめて表示。"

func marksText(marks []view.Mark) string {
	if len(marks) == 0 {
		return "印なし"
	}
	var b strings.Builder
	b.WriteString("印")
	for _, m := range marks {
		b.WriteString("「" + m.Text() + "」")
	}
	return b.String()
}
