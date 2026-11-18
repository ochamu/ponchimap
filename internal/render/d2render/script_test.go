package d2render

import (
	"context"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/ochamu/ponchimap/internal/assets"
	"github.com/ochamu/ponchimap/internal/view"
)

func testMeasurer(t *testing.T) *measurer {
	t.Helper()
	f := assets.Bundled().Fonts
	m, err := newMeasurer(f.Regular, f.Bold)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func script(t *testing.T, s *view.Scene) string {
	t.Helper()
	out, err := writeScript(context.Background(), s, testMeasurer(t), assets.Bundled())
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func wantLine(t *testing.T, script, want string) {
	t.Helper()
	for _, line := range strings.Split(script, "\n") {
		if strings.TrimSpace(line) == want {
			return
		}
	}
	t.Errorf("script has no line %q\n%s", want, withoutIcons(script))
}

func withoutIcons(script string) string {
	return regexp.MustCompile(`base64,[A-Za-z0-9+/=]+`).ReplaceAllString(script, "base64,…")
}

func TestScriptNestsGroupsAndAddressesEdgesByPath(t *testing.T) {
	out := script(t, sampleScene())
	wantLine(t, out, `"bd1": "Cloudflare アカウント A" {`)
	wantLine(t, out, `class: ["group-boundary"; "m-bd1"]`)
	wantLine(t, out, `"core": "注文の中核" {`)
	wantLine(t, out, `class: ["group-logical"; "m-core"]`)
	wantLine(t, out, `"bd1"."core"."api" -> "bd1"."core"."db": "DB接続設定（未照合）" {class: ["line-solid"; "m-e1"]}`)
	wantLine(t, out, `"bd1"."core"."api" -> "watch": "手がかり" {class: ["line-dashed"; "m-e4"]}`)

	boundary, logical, card := strings.Index(out, `"bd1": `), strings.Index(out, `"core": `), strings.Index(out, `"api" {`)
	if !(boundary < logical && logical < card) {
		t.Errorf("want the boundary to open before the logical group and the group before its card, got offsets %d, %d, %d", boundary, logical, card)
	}
}

func TestScriptDrawsCardParts(t *testing.T) {
	out := script(t, sampleScene())
	wantLine(t, out, `class: ["card"; "frame-solid"; "m-api"]`)
	wantLine(t, out, `class: ["card"; "frame-dashed"; "m-watch"]`)
	wantLine(t, out, `s: "Cloudflare Workers" {class: ["title"; "m-api"]}`)
	wantLine(t, out, `r0: "注文APIを受け付けて在庫と" {class: ["role"; "m-api"]}`)
	wantLine(t, out, `r1: "決済をまとめて処理する入口" {class: ["role"; "m-api"]}`)
	if !strings.Contains(out, `icon: "data:image/svg+xml;base64,`) {
		t.Error("cards carry no embedded icon")
	}
	if strings.Contains(out, "http") {
		t.Error("the script refers to something over http")
	}
}

func TestScriptSizesMarkPillsToTheirText(t *testing.T) {
	out := script(t, sampleScene())
	width := func(label, owner string) int {
		t.Helper()
		m := regexp.MustCompile(regexp.QuoteMeta(`: "`+label+`" {class: ["mark-unverified"; "`+owner+`"]; width: `) + `(\d+); height: \d+}`).FindStringSubmatch(out)
		if m == nil {
			t.Fatalf("no mark %q for %s\n%s", label, owner, withoutIcons(out))
		}
		n, _ := strconv.Atoi(m[1])
		return n
	}
	one, three := width("未照合", "m-db"), width("未照合 3件", "m-files")
	if !(one > 0 && three > one) {
		t.Errorf("want the counted pill wider than the plain one, got %d and %d", three, one)
	}
	wantLine(t, out, `m1: "不一致" {class: ["mark-mismatch"; "m-db"]; width: `+strconv.Itoa(one)+`; height: `+strconv.Itoa(markHeight)+`}`)
}

func TestScriptWrapsLongEdgeLabels(t *testing.T) {
	out := script(t, sampleScene())
	if !strings.Contains(out, `"登録確認メールの\n送信を依頼する呼び出し"`) {
		t.Errorf("the long edge label is not wrapped\n%s", withoutIcons(out))
	}
}

func TestScriptLegendShowsOnlyTheScenesItemsAndAlwaysTheNote(t *testing.T) {
	s := sampleScene()
	s.Legend = []view.LegendItem{view.LegendDashedLine, view.LegendMismatch, view.LegendLogical}
	out := script(t, s)
	for _, item := range view.LegendOrder {
		shown := strings.Contains(out, strconv.Quote(item.Text()))
		if want := slices.Contains(s.Legend, item); shown != want {
			t.Errorf("legend row %q shown = %v, want %v", item.Text(), shown, want)
		}
	}
	for _, note := range append([]string{view.LegendNote}, s.Notes...) {
		if !strings.Contains(out, strconv.Quote(note)) {
			t.Errorf("legend lacks the note %q", note)
		}
	}

	s.Legend, s.Notes = nil, nil
	if out := script(t, s); !strings.Contains(out, strconv.Quote(view.LegendNote)) {
		t.Error("a scene with no legend items lost the standing note")
	}
}

func TestScriptTitleIsOptional(t *testing.T) {
	s := sampleScene()
	if !strings.Contains(script(t, s), `"小規模な注文アプリ" {class: diagram-title}`) {
		t.Error("the title is not drawn")
	}
	s.Title = ""
	if strings.Contains(script(t, s), "class: diagram-title") {
		t.Error("an empty title is drawn")
	}
}

func TestScriptFollowsSceneDirection(t *testing.T) {
	s := sampleScene()
	s.Direction = view.DirDown
	wantLine(t, script(t, s), "direction: down")
}

func TestScriptEscapesLabels(t *testing.T) {
	s := sampleScene()
	s.Nodes[0].Role = `"${HOME}" \ 改行なし`
	wantLine(t, script(t, s), `r0: "\"\${HOME}\" \\ 改行なし" {class: ["role"; "m-api"]}`)
}

func TestStateOfFoldedRelationsIsDrawnApartFromElementState(t *testing.T) {
	s := sampleScene()
	for i := range s.Nodes {
		if s.Nodes[i].ID == "files" {
			s.Nodes[i].Marks = []view.Mark{
				{Kind: view.MarkUnverified, Count: 2},
				{Kind: view.MarkUnverified, Of: view.MarkOfRelation, Count: 1},
			}
		}
	}
	got := script(t, s)
	for _, want := range []string{`"未照合 2件"`, `"関係の未照合"`} {
		if !strings.Contains(got, want) {
			t.Errorf("script does not draw %s", want)
		}
	}
}
