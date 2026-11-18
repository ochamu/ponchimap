package d2render

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ochamu/ponchimap/internal/assets"
	"github.com/ochamu/ponchimap/internal/model"
	"github.com/ochamu/ponchimap/internal/render"
	"github.com/ochamu/ponchimap/internal/view"
)

var (
	withELK  = render.Options{Engine: view.EngineELK}
	withTALA = render.Options{Engine: view.EngineTALA, Seed: 1}
)

func sampleScene() *view.Scene {
	return &view.Scene{
		Title:     "小規模な注文アプリ",
		Direction: view.DirRight,
		Groups: []view.Group{
			{ID: "bd1", Kind: view.GroupBoundary, Label: "Cloudflare アカウント A"},
			{ID: "core", ParentID: "bd1", Kind: view.GroupLogical, Label: "注文の中核"},
		},
		Nodes: []view.Node{
			{ID: "api", SourceIDs: []string{"api"}, GroupID: "core", Title: "Cloudflare Workers", Role: "注文APIを受け付けて在庫と決済をまとめて処理する入口", IconKey: "compute", Frame: view.FrameSolid},
			{ID: "db", SourceIDs: []string{"db"}, GroupID: "core", Title: "Cloudflare D1", Role: "注文データの保存", IconKey: "database", Frame: view.FrameSolid,
				Marks: []view.Mark{{Kind: view.MarkUnverified, Count: 1}, {Kind: view.MarkMismatch, Count: 1}}},
			{ID: "files", SourceIDs: []string{"r2a", "r2b", "r2c"}, GroupID: "bd1", Title: "ファイル置き場", Role: "3件をまとめて表示", IconKey: "storage", Frame: view.FrameSolid,
				Marks: []view.Mark{{Kind: view.MarkUnverified, Count: 3}}},
			{ID: "mail", SourceIDs: []string{"mail"}, Title: "Resend", Role: "登録確認メールの送信", IconKey: "mail", Frame: view.FrameSolid},
			{ID: "watch", SourceIDs: []string{"watch"}, Title: "Sentry", Role: "エラー通知の候補", IconKey: "monitor", Frame: view.FrameDashed},
		},
		Edges: []view.Edge{
			{ID: "e1", SourceIDs: []string{"e1"}, From: "api", To: "db", Label: "DB接続設定（未照合）", Line: view.LineSolid},
			{ID: "e2", SourceIDs: []string{"e2"}, From: "api", To: "files", Label: "添付ファイルの読み書き", Line: view.LineSolid},
			{ID: "e3", SourceIDs: []string{"e3"}, From: "api", To: "mail", Label: "登録確認メールの送信を依頼する呼び出し", Line: view.LineSolid},
			{ID: "e4", SourceIDs: []string{"e4"}, From: "api", To: "watch", Label: "手がかり", Line: view.LineDashed},
		},
		Legend: view.LegendOrder,
		Notes:  []string{"表示設定により2件の要素を図から省いています。"},
	}
}

func draw(t *testing.T, scene *view.Scene, opts render.Options) *render.Result {
	t.Helper()
	r, err := New(assets.Bundled())
	if err != nil {
		t.Fatal(err)
	}
	res, err := r.Render(context.Background(), scene, opts)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func forEachEngine(t *testing.T, check func(t *testing.T, res *render.Result)) {
	t.Helper()
	for _, opts := range []render.Options{withELK, withTALA} {
		t.Run(string(opts.Engine), func(t *testing.T) {
			check(t, draw(t, sampleScene(), opts))
		})
	}
}

func TestTheSameSceneAndOptionsGiveTheSameBytes(t *testing.T) {
	for _, opts := range []render.Options{withELK, withTALA} {
		t.Run(string(opts.Engine), func(t *testing.T) {
			first, second := draw(t, sampleScene(), opts), draw(t, sampleScene(), opts)
			if !bytes.Equal(first.SVG, second.SVG) {
				t.Error("two renders of the same scene gave different SVG bytes")
			}
			if !bytes.Equal(first.PNG, second.PNG) {
				t.Error("two renders of the same scene gave different PNG bytes")
			}
		})
	}
}

func TestTheEngineDecidesTheLayout(t *testing.T) {
	if bytes.Equal(draw(t, sampleScene(), withELK).SVG, draw(t, sampleScene(), withTALA).SVG) {
		t.Error("ELK and TALA drew the same figure")
	}
	r, err := New(assets.Bundled())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Render(context.Background(), sampleScene(), render.Options{Engine: "graphviz", Seed: 1}); err == nil {
		t.Error("an unsupported engine rendered")
	}
}

func TestTheSeedDecidesTheTALALayout(t *testing.T) {
	scene := sceneOfSample(t, "order-service")
	seeded := func(seed int64) render.Options { return render.Options{Engine: view.EngineTALA, Seed: seed} }
	if bytes.Equal(draw(t, scene, seeded(1)).SVG, draw(t, scene, seeded(2)).SVG) {
		t.Error("TALA seeds 1 and 2 drew the same figure")
	}
	r, err := New(assets.Bundled())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Render(context.Background(), scene, render.Options{Engine: view.EngineTALA}); err == nil {
		t.Error("TALA rendered without a named seed")
	}
}

func TestELKIgnoresTheSeed(t *testing.T) {
	seeded := withELK
	seeded.Seed = 7
	if !bytes.Equal(draw(t, sampleScene(), withELK).SVG, draw(t, sampleScene(), seeded).SVG) {
		t.Error("a seed changed the ELK figure")
	}
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func sceneOfSample(t *testing.T, name string) *view.Scene {
	t.Helper()
	dir := filepath.Join("..", "..", "..", "testdata", "samples", name)
	modelPath, configPath := filepath.Join(dir, "model.json"), filepath.Join(dir, "view.yaml")
	m, diags := model.Parse(modelPath, readFile(t, modelPath))
	if diags.HasErrors() {
		t.Fatal(diags)
	}
	file, diags := view.ParseConfig(configPath, readFile(t, configPath))
	if diags.HasErrors() {
		t.Fatal(diags)
	}
	cfg, diags := file.Resolve(m)
	if len(diags) > 0 {
		t.Fatal(diags)
	}
	scene, err := view.Build(m, cfg)
	if err != nil {
		t.Fatal(err)
	}
	return scene
}

func TestEveryElementOfTheSceneIsInTheScript(t *testing.T) {
	samples, err := filepath.Glob(filepath.Join("..", "..", "..", "testdata", "samples", "*", "model.json"))
	if err != nil || len(samples) == 0 {
		t.Fatalf("found no samples: %v", err)
	}
	for _, path := range samples {
		name := filepath.Base(filepath.Dir(path))
		t.Run(name, func(t *testing.T) {
			scene := sceneOfSample(t, name)
			out := script(t, scene)
			has := func(kind, id, text string) {
				t.Helper()
				if !strings.Contains(out, text) {
					t.Errorf("the script has no %s %s: %s is missing", kind, id, text)
				}
			}
			for _, g := range scene.Groups {
				has("group", g.ID, quote(g.ID)+": "+quote(g.Label)+" {")
				has("group", g.ID, quote(ownerClass(g.ID)))
			}
			for _, n := range scene.Nodes {
				has("node", n.ID, quote(n.ID)+" {")
				has("node", n.ID, quote(ownerClass(n.ID)))
				if n.Title != "" {
					has("node", n.ID, quote(n.Title))
				}
			}
			for _, e := range scene.Edges {
				line := regexp.MustCompile(`(?m)^.*` + regexp.QuoteMeta(quote(e.From)+" -> ") + `.*` + regexp.QuoteMeta(quote(e.To)+": ") + `.*` + regexp.QuoteMeta(quote(ownerClass(e.ID))) + `.*$`)
				if !line.MatchString(out) {
					t.Errorf("the script has no line that draws edge %s from %s to %s", e.ID, e.From, e.To)
				}
			}
			drawn := regexp.MustCompile(`(?m)^\S.* -> .*class: \[`).FindAllString(out, -1)
			if len(drawn) != len(scene.Edges) {
				t.Errorf("the script draws %d edges, the scene has %d", len(drawn), len(scene.Edges))
			}
		})
	}
}

func TestSVGNeedsNothingFromTheNetwork(t *testing.T) {
	forEachEngine(t, func(t *testing.T, res *render.Result) {
		svg := string(res.SVG)
		for _, namespace := range []string{"http://www.w3.org/2000/svg", "http://www.w3.org/1999/xlink"} {
			svg = strings.ReplaceAll(svg, namespace, "")
		}
		if at := strings.Index(strings.ToLower(svg), "http"); at >= 0 {
			t.Errorf("the SVG refers to %q", svg[at:min(at+60, len(svg))])
		}
	})
}

var svgRootSize = regexp.MustCompile(`^(?:<\?xml[^>]*>)?<svg [^>]*viewBox="0 0 (\d+) (\d+)" width="(\d+)" height="(\d+)">`)

func rootSize(t *testing.T, svg []byte) (width, height int) {
	t.Helper()
	m := svgRootSize.FindSubmatch(svg)
	if m == nil {
		t.Fatalf("the SVG root declares no width and height after its viewBox: %q", svg[:min(240, len(svg))])
	}
	if string(m[1]) != string(m[3]) || string(m[2]) != string(m[4]) {
		t.Fatalf("the SVG root declares %sx%s for a viewBox of %sx%s", m[3], m[4], m[1], m[2])
	}
	width, _ = strconv.Atoi(string(m[3]))
	height, _ = strconv.Atoi(string(m[4]))
	return width, height
}

// pngIsAtItsScale checks that the PNG is the figure at PNGScale times the
// size the SVG root declares.
func pngIsAtItsScale(t *testing.T, res *render.Result) {
	t.Helper()
	width, height := rootSize(t, res.SVG)
	cfg, err := png.DecodeConfig(bytes.NewReader(res.PNG))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Width != res.PNGScale*width || cfg.Height != res.PNGScale*height {
		t.Errorf("PNG is %dx%d, want %d times the %dx%d SVG", cfg.Width, cfg.Height, res.PNGScale, width, height)
	}
}

func TestPNGIsTheFigureAtTwiceTheSizeOfTheSVG(t *testing.T) {
	forEachEngine(t, func(t *testing.T, res *render.Result) {
		if res.PNGScale != render.FullPNGScale || res.PNGLimit != "" {
			t.Fatalf("PNGScale = %d with PNGLimit = %q, want the full scale %d and no limit", res.PNGScale, res.PNGLimit, render.FullPNGScale)
		}
		pngIsAtItsScale(t, res)
	})
}

// largeScene generates a scene of n cards in four boundaries, joined as a
// tree with about one and a half relations a card, like the largest sample.
func largeScene(n int) *view.Scene {
	s := &view.Scene{Title: "生成した大きな構成", Direction: view.DirRight, Legend: view.LegendOrder}
	const boundaries = 4
	for b := range boundaries {
		s.Groups = append(s.Groups, view.Group{ID: fmt.Sprintf("bd%d", b), Kind: view.GroupBoundary, Label: fmt.Sprintf("Cloudflare アカウント %c", 'A'+b)})
	}
	card := func(i int) string { return fmt.Sprintf("n%d", i) }
	for i := range n {
		s.Nodes = append(s.Nodes, view.Node{ID: card(i), SourceIDs: []string{card(i)}, GroupID: fmt.Sprintf("bd%d", i%boundaries),
			Title: "Cloudflare Workers", Role: "注文APIを受け付けて処理する", IconKey: "compute", Frame: view.FrameSolid})
	}
	relate := func(from, to int) {
		id := fmt.Sprintf("e%d-%d", from, to)
		s.Edges = append(s.Edges, view.Edge{ID: id, SourceIDs: []string{id}, From: card(from), To: card(to), Label: "呼び出し", Line: view.LineSolid})
	}
	for i := 1; i < n; i++ {
		relate((i-1)/2, i)
		if i%2 == 0 && i > 2 {
			relate(i-1, i)
		}
	}
	return s
}

// The SVG has no largest size, the PNG does. With ELK, 50 cards make a figure
// about twice too large for a PNG at the full scale, and 200 cards one several
// times too large for a PNG at scale 1. The PNG is made after the layout, in
// the same way for either engine, so the quicker engine draws these.
func TestAFigureTooLargeForAFullPNGStillGivesTheSVG(t *testing.T) {
	for _, c := range []struct {
		name         string
		cards, scale int
	}{
		{"a PNG at scale 1", 50, 1},
		{"no PNG", 200, 0},
	} {
		t.Run(c.name, func(t *testing.T) {
			res := draw(t, largeScene(c.cards), withELK)
			if len(res.SVG) == 0 {
				t.Error("the render gave no SVG")
			}
			if res.PNGScale != c.scale {
				t.Fatalf("PNGScale = %d with PNGLimit = %q, want scale %d", res.PNGScale, res.PNGLimit, c.scale)
			}
			if !regexp.MustCompile(`^[A-Za-z0-9 -]+$`).MatchString(res.PNGLimit) {
				t.Errorf("PNGLimit = %q, want the limit in letters, digits, spaces and hyphens only", res.PNGLimit)
			}
			if c.scale == 0 {
				if res.PNG != nil {
					t.Errorf("the result has a PNG of %d bytes at scale 0", len(res.PNG))
				}
				return
			}
			pngIsAtItsScale(t, res)
		})
	}
}

func TestEveryElementOfTheSceneHasItsClassInTheSVG(t *testing.T) {
	forEachEngine(t, func(t *testing.T, res *render.Result) {
		drawn := map[string]bool{}
		for _, m := range regexp.MustCompile(`class="([^"]*)"`).FindAllSubmatch(res.SVG, -1) {
			for _, name := range strings.Fields(string(m[1])) {
				drawn[name] = true
			}
		}
		for _, id := range []string{"bd1", "core", "api", "db", "files", "mail", "watch", "e1", "e2", "e3", "e4"} {
			if class := ownerClass(id); !drawn[class] {
				t.Errorf("class %q for %q is not on any SVG element", class, id)
			}
		}
	})
}

func TestDashedMeansDashedInTheSVG(t *testing.T) {
	forEachEngine(t, func(t *testing.T, res *render.Result) {
		dashed := func(classes string) bool {
			t.Helper()
			m := regexp.MustCompile(`<g class="[^"]* ` + regexp.QuoteMeta(classes) + `"[^>]*>.*?</g>`).Find(res.SVG)
			if m == nil {
				t.Fatalf("no SVG element with classes %q", classes)
			}
			return bytes.Contains(m, []byte("stroke-dasharray"))
		}
		for classes, want := range map[string]bool{
			"card frame-dashed m-watch": true, "card frame-solid m-mail": false,
			"line-dashed m-e4": true, "line-solid m-e1": false,
		} {
			if got := dashed(classes); got != want {
				t.Errorf("%q dashed = %v, want %v", classes, got, want)
			}
		}
	})
}

func TestEdgeLabelsAreUpright(t *testing.T) {
	forEachEngine(t, func(t *testing.T, res *render.Result) {
		if bytes.Contains(res.SVG, []byte("text-italic")) {
			t.Error("the SVG draws italic text")
		}
	})
}

func TestMissingGlyphFailsBeforeDrawing(t *testing.T) {
	inRole, inTitle := sampleScene(), sampleScene()
	inRole.Nodes[1].Role = "保存先 🚀"
	inTitle.Title = "注文アプリ 🚀"
	r, err := New(assets.Bundled())
	if err != nil {
		t.Fatal(err)
	}
	for name, c := range map[string]struct {
		scene   *view.Scene
		sceneID string
	}{
		"in the role of a node":     {inRole, "db"},
		"in the title of the scene": {inTitle, ""},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := r.Render(context.Background(), c.scene, withELK)
			var missing *render.MissingGlyphError
			if !errors.As(err, &missing) {
				t.Fatalf("err = %v, want a MissingGlyphError", err)
			}
			if missing.Rune != '🚀' || missing.SceneID != c.sceneID {
				t.Errorf("the error names U+%04X in the element %q, want U+1F680 in %q", missing.Rune, missing.SceneID, c.sceneID)
			}
			if strings.ContainsAny(err.Error(), "保存先注文アプリ🚀") {
				t.Errorf("the error shows the text of the figure: %v", err)
			}
		})
	}
}

func TestTheRendererDrawsWithTheIconsOfItsSet(t *testing.T) {
	compute := []byte(base64.StdEncoding.EncodeToString(assets.Bundled().Icon("compute")))
	withIcon, withoutIcon := sampleScene(), sampleScene()
	withoutIcon.Nodes[0].IconKey = "queue"
	if !bytes.Contains(draw(t, withIcon, withELK).SVG, compute) {
		t.Error("a scene with a compute card is drawn without the compute icon of the set")
	}
	if bytes.Contains(draw(t, withoutIcon, withELK).SVG, compute) {
		t.Error("a scene without a compute card is drawn with the compute icon of the set")
	}
}

// D2 writes into the font bytes it is given.
func TestARendererLeavesTheFontsOfItsSetAsTheyAreInTheFiles(t *testing.T) {
	set := assets.Bundled()
	if _, err := New(set); err != nil {
		t.Fatal(err)
	}
	for name, font := range map[string][]byte{"BIZUDPGothic-Regular.ttf": set.Fonts.Regular, "BIZUDPGothic-Bold.ttf": set.Fonts.Bold} {
		file, err := os.ReadFile(filepath.Join("..", "..", "assets", "fonts", name))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(font, file) {
			t.Errorf("after a Renderer was made, %s of the set is not the bytes of the file", name)
		}
	}
}

func TestOnlyALimitOfD2IsPassedOnAsThePNGLimit(t *testing.T) {
	for _, limit := range []string{
		"frame width 40000 exceeds limit 32768",
		"frame height 40000 exceeds limit 32768",
		"frame pixels exceed limit 67108864",
		"peak offscreen pixel storage 600000000 bytes exceeds limit 536870912",
		"even-odd clip work 300000000 exceeds limit 250000000",
		"scanline work 5000000000 exceeds limit 4000000000",
	} {
		if got := pngLimit(errors.New("d2raster: " + limit)); got != limit {
			t.Errorf("pngLimit of D2's %q = %q, want the limit", limit, got)
		}
	}
	for _, quoting := range []string{
		`d2raster: node "n1" filter 0 has unsupported type X`,
		`d2raster: clip: something "label"`,
	} {
		if got := pngLimit(errors.New(quoting)); got != "" {
			t.Errorf("pngLimit of %q = %q, want nothing", quoting, got)
		}
	}
}

func TestAwkwardIDsAndLabelsRender(t *testing.T) {
	s := &view.Scene{
		Groups: []view.Group{{ID: "style", Kind: view.GroupBoundary, Label: `"引用" と ${変数} と \ 記号`}},
		Nodes: []view.Node{
			{ID: "label", SourceIDs: []string{"label"}, GroupID: "style", Title: "near", Role: "${x}", IconKey: "no-such-icon", Frame: view.FrameSolid},
			{ID: "class", SourceIDs: []string{"class"}, Title: "a.b", Frame: view.FrameDashed},
		},
		Edges:  []view.Edge{{ID: "direction", SourceIDs: []string{"direction"}, From: "label", To: "class", Line: view.LineSolid}},
		Legend: []view.LegendItem{view.LegendSolidLine},
	}
	for _, opts := range []render.Options{withELK, withTALA} {
		if res := draw(t, s, opts); !bytes.Contains(res.SVG, []byte("${x}")) {
			t.Errorf("%s: the role text ${x} is not drawn literally", opts.Engine)
		}
	}
}

// D2 takes a container named like one of its board keywords for a set of
// boards even when the key is quoted: it refuses a relation into it and
// repeats the rest of the figure inside it.
func TestAnElementNamedLikeAD2BoardKeywordIsDrawnLikeAnyOther(t *testing.T) {
	scene := func(group, inside, outside string) *view.Scene {
		unverified := []view.Mark{{Kind: view.MarkUnverified, Count: 1}}
		return &view.Scene{
			Groups: []view.Group{{ID: group, Kind: view.GroupLogical, Label: "注文の中核"}},
			Nodes: []view.Node{
				{ID: inside, SourceIDs: []string{inside}, GroupID: group, Title: "Cloudflare Workers", Role: "注文の受付", IconKey: "compute", Frame: view.FrameSolid, Marks: unverified},
				{ID: "db", SourceIDs: []string{"db"}, GroupID: group, Title: "Cloudflare D1", Role: "注文の保存先", IconKey: "database", Frame: view.FrameSolid},
				{ID: outside, SourceIDs: []string{outside}, Title: "Resend", Role: "確認メールの送信", IconKey: "mail", Frame: view.FrameSolid, Marks: unverified},
			},
			Edges: []view.Edge{
				{ID: "e1", SourceIDs: []string{"e1"}, From: inside, To: "db", Label: "注文を書き込む", Line: view.LineSolid},
				{ID: "e2", SourceIDs: []string{"e2"}, From: outside, To: inside, Label: "受信を知らせる", Line: view.LineSolid},
			},
			Legend: view.LegendOrder,
		}
	}
	// A card named steps gains only an icon and one named layers only a text
	// element, so the elements are counted as well as the texts.
	drawn := []string{"注文の中核", "Cloudflare Workers", "注文の受付", "Cloudflare D1", "注文の保存先", "Resend", "確認メールの送信", "注文を書き込む", "受信を知らせる", "<text", "<image"}
	for _, opts := range []render.Options{withELK, withTALA} {
		plain := draw(t, scene("core", "api", "mail"), opts).SVG
		for _, word := range []string{"layers", "scenarios", "steps"} {
			for kind, named := range map[string]*view.Scene{
				"group":               scene(word, "api", "mail"),
				"card in a group":     scene("core", word, "mail"),
				"card outside groups": scene("core", "api", word),
			} {
				t.Run(fmt.Sprintf("a %s named %s with %s", kind, word, opts.Engine), func(t *testing.T) {
					res := draw(t, named, opts)
					for _, text := range drawn {
						want := bytes.Count(plain, []byte(text))
						if got := bytes.Count(res.SVG, []byte(text)); got != want || want == 0 {
							t.Errorf("the SVG has %q %d times, the same scene with plain IDs has it %d times", text, got, want)
						}
					}
					if class := ownerClass(word); !bytes.Contains(res.SVG, []byte(" "+class+`"`)) {
						t.Errorf("the class %s of %s is on no SVG element", class, word)
					}
				})
			}
		}
	}
}

// renderError gives the error of a render. It gives up after a minute, so
// that a render that ignores its context fails one test instead of running
// into the timeout of the whole test binary.
func renderError(t *testing.T, ctx context.Context, scene *view.Scene) error {
	t.Helper()
	r, err := New(assets.Bundled())
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		_, err := r.Render(ctx, scene, withELK)
		done <- err
	}()
	select {
	case err := <-done:
		return err
	case <-time.After(time.Minute):
		t.Fatal("the render is still running after a minute")
		return nil
	}
}

func TestARenderWithACancelledContextIsCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := renderError(t, ctx, sampleScene()); !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
}

func TestACancelledRenderDoesNotWaitForALongLabelToWrap(t *testing.T) {
	// Wrapped to the end, a label of this length keeps a processor busy for
	// far longer than any test run.
	long := strings.Repeat("注文の配送状況を画面へ通知する", 1000)
	inRole, inRelation := sampleScene(), sampleScene()
	inRole.Nodes[0].Role = long
	inRelation.Edges[0].Label = long

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	for name, scene := range map[string]*view.Scene{"role": inRole, "relation label": inRelation} {
		t.Run("cancelled before a long "+name, func(t *testing.T) {
			if err := renderError(t, cancelled, scene); !errors.Is(err, context.Canceled) {
				t.Errorf("err = %v, want context.Canceled", err)
			}
		})
	}
	t.Run("deadline passes while a long role wraps", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()
		if err := renderError(t, ctx, inRole); !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("err = %v, want context.DeadlineExceeded", err)
		}
	})
}
