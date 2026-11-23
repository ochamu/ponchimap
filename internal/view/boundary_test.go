package view

import (
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/ochamu/ponchimap/internal/diag"
	"github.com/ochamu/ponchimap/internal/model"
)

func everyModel(t *testing.T) []string {
	t.Helper()
	var paths []string
	for _, pattern := range []string{"samples/*/model.json", "small-models/*.json", "boundaries/*.json"} {
		found, err := filepath.Glob(filepath.Join("..", "..", "testdata", pattern))
		if err != nil {
			t.Fatal(err)
		}
		paths = append(paths, found...)
	}
	if len(paths) < 7 {
		t.Fatalf("found %d model files, want the samples and the small models at least", len(paths))
	}
	return paths
}

func newcomer(t *testing.T, provider string) model.Node {
	t.Helper()
	extra := `,{"key":"scope.owner","value":{"kind":"account","id":"zz-newcomer-account"}},
		{"key":"region","value":"zz-newcomer-region"}`
	raw := modelOf(deployedNode("zz-newcomer", provider, extra), "")
	m, diags := model.Parse("newcomer", []byte(raw))
	if diags.HasErrors() {
		t.Fatal(diags)
	}
	return m.Nodes[0]
}

// chains gives, for each node of the model that the scene draws, the real
// boundaries around it from the outside in, each written by show.
func chains(s *Scene, show func(Group) string) map[string]string {
	groups := map[string]Group{}
	for _, g := range s.Groups {
		groups[g.ID] = g
	}
	out := map[string]string{}
	for _, n := range s.Nodes {
		var chain []string
		for at := n.GroupID; at != ""; at = groups[at].ParentID {
			if g := groups[at]; g.Kind == GroupBoundary {
				chain = append(chain, show(g))
			}
		}
		slices.Reverse(chain)
		for _, id := range n.SourceIDs {
			out[id] = strings.Join(chain, " > ")
		}
	}
	return out
}

func boundaries(s *Scene) map[string]string {
	return chains(s, func(g Group) string { return g.ID + " " + g.Label })
}

func boundaryNames(s *Scene) map[string]string {
	return chains(s, func(g Group) string { return g.Label })
}

// twinOf is a node that the evidence puts in every boundary n is in.
func twinOf(n model.Node) model.Node {
	twin := n
	twin.ID = "zz-twin"
	twin.Evidence = slices.Clone(n.Evidence)
	for i := range twin.Evidence {
		twin.Evidence[i].ID = fmt.Sprintf("zz-twin-ev%d", i)
	}
	return twin
}

func without(m *model.Model, id string) *model.Model {
	out := *m
	out.Nodes = slices.DeleteFunc(slices.Clone(m.Nodes), func(n model.Node) bool { return n.ID == id })
	out.Edges = slices.DeleteFunc(slices.Clone(m.Edges), func(e model.Edge) bool { return e.From == id || e.To == id })
	return &out
}

func TestBoundaryIDsStayStableWhenNodesAreInsertedHiddenOrAppended(t *testing.T) {
	for _, path := range everyModel(t) {
		t.Run(path, func(t *testing.T) {
			m := loadModel(t, path)
			before := boundaries(scene(t, m, &Config{}))
			providers := []string{"zz-provider"}
			if len(m.Nodes) > 0 {
				providers = append(providers, m.Nodes[0].Provider)
			}
			for _, provider := range providers {
				newNode := newcomer(t, provider)
				for _, at := range []int{0, len(m.Nodes) / 2, len(m.Nodes)} {
					t.Run(fmt.Sprintf("insert %s at %d", provider, at), func(t *testing.T) {
						grown := *m
						grown.Nodes = slices.Insert(slices.Clone(m.Nodes), at, newNode)
						wantBoundaries(t, scene(t, &grown, &Config{}), before)
					})
				}
			}
			for _, n := range m.Nodes {
				t.Run("hide "+n.ID, func(t *testing.T) {
					want := maps.Clone(before)
					delete(want, n.ID)
					wantBoundaries(t, scene(t, m, &Config{Hidden: []string{n.ID}}), want)
				})
				t.Run("append twin of "+n.ID, func(t *testing.T) {
					grown := *m
					grown.Nodes = append(slices.Clone(m.Nodes), twinOf(n))
					wantBoundaries(t, scene(t, &grown, &Config{}), before)
				})
			}
		})
	}
}

// A person names the accounts of a figure once, through a node of each, and
// investigations then add and remove the other nodes. The names have to hold
// whichever node the model puts first in a boundary, so the view config names
// a boundary by a node in it and never by the ID the figure gives it.
func TestANameStaysWithItsBoundaryWhileOtherNodesComeAndGo(t *testing.T) {
	for _, path := range everyModel(t) {
		m := loadModel(t, path)
		plain := scene(t, m, &Config{})
		for _, named := range m.Nodes {
			c := &Config{}
			for _, g := range boundariesOf(t, plain, named.ID) {
				c.Boundaries = append(c.Boundaries, BoundaryAlias{Member: named.ID, Level: string(g.Level), Label: fmt.Sprintf("名前%d", len(c.Boundaries)+1)})
			}
			want := boundaryNames(scene(t, m, c))[named.ID]
			if strings.Count(want, "名前") != len(c.Boundaries) {
				t.Fatalf("%s: the boundaries of %s are named %q, want the %d names of the view config", path, named.ID, want, len(c.Boundaries))
			}
			for _, other := range m.Nodes {
				grown := *m
				grown.Nodes = slices.Insert(slices.Clone(m.Nodes), 0, twinOf(other))
				if got := boundaryNames(scene(t, &grown, c))[named.ID]; got != want {
					t.Errorf("%s, a node beside %s added first: the boundaries of %s were named %q and are named %q", path, other.ID, named.ID, want, got)
				}
				if other.ID == named.ID {
					continue
				}
				if got := boundaryNames(scene(t, without(m, other.ID), c))[named.ID]; got != want {
					t.Errorf("%s, %s removed: the boundaries of %s were named %q and are named %q", path, other.ID, named.ID, want, got)
				}
				hiding := *c
				hiding.Hidden = []string{other.ID}
				if got := boundaryNames(scene(t, m, &hiding))[named.ID]; got != want {
					t.Errorf("%s, %s hidden: the boundaries of %s were named %q and are named %q", path, other.ID, named.ID, want, got)
				}
			}
		}
	}
}

// disagreeing is the model with one more piece of evidence on the node, a copy
// of one it has that gives another value for the level: what a file left
// behind after a move says, beside what was deployed.
func disagreeing(t *testing.T, m *model.Model, id string, level Level) *model.Model {
	t.Helper()
	key := levels[slices.IndexFunc(levels, func(r levelRule) bool { return r.level == level })].key
	other := map[Level]any{
		LevelOwner:   map[string]any{"kind": "account", "id": "zz-other-account"},
		LevelRegion:  "zz-other-region",
		LevelNetwork: "zz-other-network",
	}[level]
	out := *m
	out.Nodes = slices.Clone(m.Nodes)
	n := &out.Nodes[slices.IndexFunc(out.Nodes, func(n model.Node) bool { return n.ID == id })]
	for _, ev := range n.Evidence {
		at := slices.IndexFunc(ev.Supports, func(s model.Support) bool { return s.Key == key })
		if at < 0 || !slices.Contains(directKinds, ev.Kind) {
			continue
		}
		ev.ID = "zz-other-evidence"
		ev.Supports = slices.Clone(ev.Supports)
		ev.Supports[at].Value = other
		n.Evidence = append(slices.Clone(n.Evidence), ev)
		return &out
	}
	t.Fatalf("no direct evidence of %s states %s", id, key)
	return nil
}

// earlier is the model in which the evidence that disagreeing added to the
// node speaks of an earlier time: the record of where a resource was before it
// was moved, which a model keeps as history and does not compare with the
// present.
func earlier(m *model.Model, id string) *model.Model {
	out := *m
	out.Nodes = slices.Clone(m.Nodes)
	n := &out.Nodes[slices.IndexFunc(out.Nodes, func(n model.Node) bool { return n.ID == id })]
	n.Evidence = slices.Clone(n.Evidence)
	n.Evidence[len(n.Evidence)-1].ValidTime = "2026-10-01T00:00:00Z"
	return &out
}

// A node drawn outside a boundary that its evidence names reads, in the
// figure, as a node that is not in it. Something has to say otherwise: the
// mismatch mark on the node, or a warning about it.
func wantOutsideSaid(t *testing.T, what string, m *model.Model, s *Scene) {
	t.Helper()
	in := deriveBoundaries(m).in
	for _, n := range s.Nodes {
		marked := slices.ContainsFunc(n.Marks, func(m Mark) bool { return m.Kind == MarkMismatch && m.Of == "" })
		for _, id := range n.SourceIDs {
			outside := slices.ContainsFunc(in[id], func(in Membership) bool { return in.Outside != "" })
			warned := slices.ContainsFunc(s.Warnings, func(w diag.Diagnostic) bool { return w.TargetID == id && w.Location.Pointer == "" })
			if outside && !marked && !warned {
				t.Errorf("%s: %s is left outside a boundary with no mark and no warning; it is in %+v", what, id, in[id])
			}
		}
	}
}

// Which of two regions a node is in is not for the figure to settle, and
// neither is which region its network is in. A network drawn for the node
// alone, outside the region, would show one network as two.
func TestANodeIsInNoBoundaryInsideALevelItsEvidenceDisagreesAbout(t *testing.T) {
	for _, path := range everyModel(t) {
		m := loadModel(t, path)
		plain := scene(t, m, &Config{})
		for _, n := range m.Nodes {
			var outside []string
			for _, g := range boundariesOf(t, plain, n.ID) {
				contested := disagreeing(t, m, n.ID, g.Level)
				for when, changed := range map[string]*model.Model{"now": contested, "at an earlier time": earlier(contested, n.ID)} {
					what := fmt.Sprintf("%s, the evidence of %s disagrees about its %s %s", path, n.ID, g.Level, when)
					s := scene(t, changed, &Config{})
					if got, want := boundaries(s)[n.ID], strings.Join(outside, " > "); got != want {
						t.Errorf("%s: its boundaries are %q, want %q", what, got, want)
					}
					wantOutsideSaid(t, what, changed, s)
				}
				outside = append(outside, g.ID+" "+g.Label)
			}
		}
	}
}

func boundaryCase(t *testing.T, name string) *model.Model {
	t.Helper()
	return loadModel(t, filepath.Join("..", "..", "testdata", "boundaries", name))
}

func wantBoundaries(t *testing.T, s *Scene, want map[string]string) {
	t.Helper()
	got := boundaries(s)
	for id, chain := range want {
		if actual, ok := got[id]; !ok {
			t.Errorf("node %s is missing from the figure", id)
		} else if actual != chain {
			t.Errorf("the boundaries of %s are %q, want %q", id, actual, chain)
		}
	}
}

func groupOf(t *testing.T, s *Scene, id string) Group {
	t.Helper()
	for _, g := range s.Groups {
		if g.ID == id {
			return g
		}
	}
	t.Fatalf("scene has no group %q; groups = %+v", id, s.Groups)
	return Group{}
}

func warned(t *testing.T, s *Scene) []string {
	t.Helper()
	out := []string{}
	for _, w := range s.Warnings {
		if w.Severity != diag.Warning || w.Message == "" || w.Hint == "" || w.Location.Path != "" || w.Location.Line != 0 {
			t.Errorf("warning %+v: want a warning with a message and a hint, and no file or line", w)
		}
		out = append(out, w.Code+" "+w.Location.Pointer+" "+w.TargetID)
	}
	return out
}

func wantWarned(t *testing.T, s *Scene, want ...string) {
	t.Helper()
	if want == nil {
		want = []string{}
	}
	if got := warned(t, s); !slices.Equal(got, want) {
		t.Errorf("warnings = %q, want %q", got, want)
	}
}

const (
	prodAccount = "bd_o_web AWS アカウント"
	prodRegion  = prodAccount + " > bd_r_web リージョン ap-northeast-1"
	prodNetwork = prodRegion + " > bd_n_web ネットワーク"
	devNetwork  = "bd_o_web-dev AWS アカウント > bd_r_web-dev リージョン ap-northeast-1 > bd_n_web-dev ネットワーク"
)

// accountNames gives the two accounts of the case names of their own, so that a
// test about something else gets no warning about boundaries under one name.
var accountNames = []BoundaryAlias{{Member: "web", Level: "owner", Label: "本番"}, {Member: "web-dev", Level: "owner", Label: "開発"}}

// The boundaries come from the evidence of the nodes, one inside the other,
// and each is named after the first node that the model puts in it.
func TestANetworkIsABoundaryInsideItsRegion(t *testing.T) {
	s := scene(t, boundaryCase(t, "two-accounts.json"), &Config{})
	wantWellFormed(t, s)
	wantBoundaries(t, s, map[string]string{
		"web":     prodNetwork,
		"db":      prodNetwork,
		"assets":  prodRegion,
		"web-dev": devNetwork,
		"db-dev":  devNetwork,
		"edge":    "bd_o_edge Cloudflare アカウント",
		"mail":    "",
	})
	for id, level := range map[string]Level{"bd_o_web": LevelOwner, "bd_r_web": LevelRegion, "bd_n_web": LevelNetwork} {
		if got := groupOf(t, s, id).Level; got != level {
			t.Errorf("level of %s = %q, want %q", id, got, level)
		}
	}
}

func TestABoundaryIsNamedThroughANodeThatIsInIt(t *testing.T) {
	s := scene(t, boundaryCase(t, "two-accounts.json"), &Config{
		Boundaries: append(slices.Clone(accountNames), BoundaryAlias{Member: "db", Level: "network", Label: "本番VPC"}, BoundaryAlias{Member: "assets", Level: "region", Label: "東京"}),
		Hidden:     []string{"db"},
	})
	wantBoundaries(t, s, map[string]string{
		"web":    "bd_o_web 本番 > bd_r_web 東京 > bd_n_web 本番VPC",
		"assets": "bd_o_web 本番 > bd_r_web 東京",
	})
	wantWarned(t, s)
}

// The figure names a boundary after its first node, so a node added before
// that one changes the ID in the figure. The display name is not lost with it.
func TestANameStaysWhenANodeIsAddedBeforeTheFirstNodeOfItsBoundary(t *testing.T) {
	m := boundaryCase(t, "two-accounts.json")
	c := &Config{Boundaries: append(slices.Clone(accountNames), BoundaryAlias{Member: "web", Level: "network", Label: "本番VPC"})}
	wantBoundaries(t, scene(t, m, c), map[string]string{"web": "bd_o_web 本番 > bd_r_web リージョン ap-northeast-1 > bd_n_web 本番VPC"})

	web := m.Nodes[slices.IndexFunc(m.Nodes, func(n model.Node) bool { return n.ID == "web" })]
	grown := *m
	grown.Nodes = slices.Insert(slices.Clone(m.Nodes), 0, twinOf(web))
	s := scene(t, &grown, c)
	wantBoundaries(t, s, map[string]string{
		"web":     "bd_o_zz-twin 本番 > bd_r_zz-twin リージョン ap-northeast-1 > bd_n_zz-twin 本番VPC",
		"db":      "bd_o_zz-twin 本番 > bd_r_zz-twin リージョン ap-northeast-1 > bd_n_zz-twin 本番VPC",
		"web-dev": "bd_o_web-dev 開発 > bd_r_web-dev リージョン ap-northeast-1 > bd_n_web-dev ネットワーク",
	})
	wantWarned(t, s)
}

// The first node of the production account, region, and network gives all
// three their names, and then a declaration is found that puts it in another
// region. The other nodes of the network have not moved, so the network is
// still drawn once, in its region, and the two names that can no longer be
// placed are reported.
func TestTheNamesOfANodeWhoseRegionIsContestedAreLeftOutInsideIt(t *testing.T) {
	m := disagreeing(t, boundaryCase(t, "two-accounts.json"), "web", LevelRegion)
	s := scene(t, m, &Config{Boundaries: append(slices.Clone(accountNames),
		BoundaryAlias{Member: "web", Level: "region", Label: "東京"},
		BoundaryAlias{Member: "web", Level: "network", Label: "本番VPC"},
	)})
	wantWellFormed(t, s)
	wantBoundaries(t, s, map[string]string{
		"web":    "bd_o_web 本番",
		"db":     "bd_o_web 本番 > bd_r_db リージョン ap-northeast-1 > bd_n_db ネットワーク",
		"assets": "bd_o_web 本番 > bd_r_db リージョン ap-northeast-1",
	})
	wantWarned(t, s, "view.boundary_unbacked /boundaries/2 web", "view.boundary_unbacked /boundaries/3 web")
}

// The same disagreement on a node beside the first: it leaves the network of
// the others as it was and is not drawn in a network of its own.
func TestANodeWhoseRegionIsContestedLeavesItsNetworkToTheOthers(t *testing.T) {
	m := disagreeing(t, boundaryCase(t, "two-accounts.json"), "db", LevelRegion)
	s := scene(t, m, &Config{Boundaries: append(slices.Clone(accountNames),
		BoundaryAlias{Member: "web", Level: "region", Label: "東京"},
		BoundaryAlias{Member: "web", Level: "network", Label: "本番VPC"},
	)})
	wantWellFormed(t, s)
	wantBoundaries(t, s, map[string]string{
		"web":    "bd_o_web 本番 > bd_r_web 東京 > bd_n_web 本番VPC",
		"db":     "bd_o_web 本番",
		"assets": "bd_o_web 本番 > bd_r_web 東京",
	})
	wantWarned(t, s)
	networks := 0
	for _, g := range s.Groups {
		if g.Level == LevelNetwork {
			networks++
		}
	}
	if networks != 2 {
		t.Errorf("the figure has %d networks, want the one of each account", networks)
	}
}

// Two nodes of one account name the same network, and one of them names no
// region. The network is drawn once, in the region of the other, and the
// figure says nothing false about the node without a region: it is drawn in
// its account, and a warning says why it is not in the network.
func TestANodeWithoutARegionLeavesItsNetworkToTheOthersAndIsReported(t *testing.T) {
	m := boundaryCase(t, "two-accounts.json")
	db := &m.Nodes[slices.IndexFunc(m.Nodes, func(n model.Node) bool { return n.ID == "db" })]
	for i := range db.Evidence {
		db.Evidence[i].Supports = slices.DeleteFunc(db.Evidence[i].Supports, func(s model.Support) bool { return s.Key == "region" })
	}
	s := scene(t, m, &Config{Boundaries: append(slices.Clone(accountNames), BoundaryAlias{Member: "db", Level: "network", Label: "本番VPC"})})
	wantWellFormed(t, s)
	wantBoundaries(t, s, map[string]string{
		"web": "bd_o_web 本番 > bd_r_web リージョン ap-northeast-1 > bd_n_web ネットワーク",
		"db":  "bd_o_web 本番",
	})
	wantWarned(t, s, "view.network_without_region  db", "view.boundary_unbacked /boundaries/2 db")
	networks := 0
	for _, g := range s.Groups {
		if g.Level == LevelNetwork {
			networks++
		}
	}
	if networks != 2 {
		t.Errorf("the figure has %d networks, want the one of each account", networks)
	}

	wantWarned(t, scene(t, m, &Config{Boundaries: accountNames, Hidden: []string{"db"}}))
}

// A resource was moved to another region, and the model keeps the record of
// where it was, with the time that record speaks of. The two regions are not
// compared, so the node carries no mismatch mark, and the figure alone would
// say that it is in no region and no network. A warning says what happened.
func TestANodeLeftOutsideABoundaryWithNoMarkIsReported(t *testing.T) {
	contested := disagreeing(t, boundaryCase(t, "two-accounts.json"), "db", LevelRegion)
	m := earlier(contested, "db")
	s := scene(t, m, &Config{Boundaries: accountNames})
	wantWellFormed(t, s)
	wantBoundaries(t, s, map[string]string{
		"web": "bd_o_web 本番 > bd_r_web リージョン ap-northeast-1 > bd_n_web ネットワーク",
		"db":  "bd_o_web 本番",
	})
	if marks := node(t, s, "db").Marks; len(marks) != 0 {
		t.Fatalf("db carries the marks %+v, want a record of an earlier time to leave none", marks)
	}
	wantWarned(t, s, "view.boundary_contested  db")

	wantWarned(t, scene(t, m, &Config{Boundaries: accountNames, Hidden: []string{"db"}}))

	s = scene(t, contested, &Config{Boundaries: accountNames})
	if marks := node(t, s, "db").Marks; !slices.ContainsFunc(marks, func(m Mark) bool { return m.Kind == MarkMismatch }) {
		t.Fatalf("db carries the marks %+v, want the mismatch of two regions at one time", marks)
	}
	wantWarned(t, s)
}

// boundariesOf gives the real boundaries that the scene draws the node id in,
// from the outside in.
func boundariesOf(t *testing.T, s *Scene, id string) []Group {
	t.Helper()
	var out []Group
	for at := node(t, s, id).GroupID; at != ""; {
		g := groupOf(t, s, at)
		if g.Kind == GroupBoundary {
			out = append(out, g)
		}
		at = g.ParentID
	}
	slices.Reverse(out)
	return out
}

// The report traces a boundary and the membership of each element back to
// evidence, for elements the figure leaves out too.
func TestEveryBoundarySaysWhichEvidencePutsItsNodesInIt(t *testing.T) {
	s := scene(t, boundaryCase(t, "two-accounts.json"), &Config{Hidden: []string{"db"}})
	if got := boundaries(s)["mail"]; got != "" {
		t.Errorf("mail has no evidence of an owner, and is in %q", got)
	}
	for _, id := range []string{"bd_o_web", "bd_r_web"} {
		if got := groupOf(t, s, id).EvidenceIDs; !slices.Contains(got, "ev-db") {
			t.Errorf("%s rests on %v, want the evidence of the hidden db among them", id, got)
		}
	}
	if got, want := groupOf(t, s, "bd_n_web").EvidenceIDs, []string{"ev-web", "ev-db"}; !slices.Equal(got, want) {
		t.Errorf("the network rests on %v, want %v", got, want)
	}
}

// A resource may be attached to several networks. Which one to draw it in is
// not for the tool to pick, and two networks are no discrepancy.
func TestANodeOfTwoNetworksIsDrawnInNeither(t *testing.T) {
	s := scene(t, boundaryCase(t, "two-networks.json"), &Config{})
	region := "bd_o_api AWS アカウント > bd_r_api リージョン ap-northeast-1"
	wantBoundaries(t, s, map[string]string{"api": region + " > bd_n_api ネットワーク", "gateway": region})
	wantNode(t, s, "gateway", FrameSolid)
	wantWarned(t, s, "view.boundary_contested  gateway")
}

// The levels nest in one order, so a network that reaches into two regions is
// one boundary in each.
func TestANetworkOfTwoRegionsIsABoundaryInEach(t *testing.T) {
	s := scene(t, boundaryCase(t, "network-in-two-regions.json"), &Config{})
	project := "bd_o_tokyo Google Cloud プロジェクト"
	wantBoundaries(t, s, map[string]string{
		"tokyo": project + " > bd_r_tokyo リージョン asia-northeast1 > bd_n_tokyo ネットワーク",
		"osaka": project + " > bd_r_osaka リージョン asia-northeast2 > bd_n_osaka ネットワーク",
	})
}

func nodeGroups(s *Scene) map[string]string {
	out := map[string]string{}
	for _, n := range s.Nodes {
		out[n.ID] = n.GroupID
	}
	return out
}

func TestAGroupThatSpansBoundariesIsDrawnInEach(t *testing.T) {
	s := scene(t, boundaryCase(t, "two-accounts.json"), &Config{
		Boundaries: accountNames,
		Groups:     []GroupConfig{{ID: "g-data", Label: "データ", Members: []string{"db", "db-dev", "assets"}}},
	})
	wantWellFormed(t, s)
	for id, parent := range map[string]string{"g-data": "bd_n_web", "g-data_2": "bd_n_web-dev", "g-data_3": "bd_r_web"} {
		g := groupOf(t, s, id)
		if g.Kind != GroupLogical || g.Label != "データ" || g.ParentID != parent {
			t.Errorf("part %s = %+v, want a logical group データ of g-data inside %s", id, g, parent)
		}
	}
	got := nodeGroups(s)
	for id, group := range map[string]string{"db": "g-data", "db-dev": "g-data_2", "assets": "g-data_3", "web": "bd_n_web"} {
		if got[id] != group {
			t.Errorf("%s is drawn in %q, want %q", id, got[id], group)
		}
	}
	if len(s.Nodes) != 7 {
		t.Errorf("the figure draws %d nodes, want all 7", len(s.Nodes))
	}
	wantWarned(t, s, "view.split /groups/0 g-data")
	if !slices.Contains(s.Notes, splitNote) {
		t.Errorf("notes = %q, want the note that says why one name is drawn twice", s.Notes)
	}
}

func TestAGroupInsideOneBoundaryIsDrawnOnce(t *testing.T) {
	s := scene(t, boundaryCase(t, "two-accounts.json"), &Config{
		Boundaries: accountNames,
		Groups:     []GroupConfig{{ID: "g-app", Label: "受付と保存", Members: []string{"web", "db"}}},
	})
	if g := groupOf(t, s, "g-app"); g.ParentID != "bd_n_web" {
		t.Errorf("group = %+v, want g-app inside the network", g)
	}
	for _, g := range s.Groups {
		if g.Kind == GroupLogical && g.ID != "g-app" {
			t.Errorf("unexpected logical group %+v", g)
		}
	}
	wantWarned(t, s)
	if slices.Contains(s.Notes, splitNote) {
		t.Errorf("notes = %q; nothing was drawn in parts", s.Notes)
	}
}

func TestAnAggregateThatSpansBoundariesIsFoldedInEach(t *testing.T) {
	s := scene(t, boundaryCase(t, "two-accounts.json"), &Config{
		Boundaries: accountNames,
		Aggregates: []AggregateConfig{{ID: "a-store", Label: "受付と保存", Members: []string{"web", "db", "db-dev"}}},
	})
	wantWellFormed(t, s)
	first, second := node(t, s, "a-store"), node(t, s, "a-store_2")
	if first.GroupID != "bd_n_web" || !slices.Equal(first.SourceIDs, []string{"web", "db"}) || !slices.Equal(first.FoldedEdgeIDs, []string{"e-web-db"}) {
		t.Errorf("first part = %+v, want web and db folded inside the network of the first account", first)
	}
	if second.GroupID != "bd_n_web-dev" || !slices.Equal(second.SourceIDs, []string{"db-dev"}) || second.Title != first.Title {
		t.Errorf("second part = %+v, want db-dev folded under the same name inside the network of the second account", second)
	}
	if e := edge(t, s, "e-webdev-dbdev"); e.From != "web-dev" || e.To != "a-store_2" {
		t.Errorf("the relation to db-dev runs from %s to %s, want it to end at the second part", e.From, e.To)
	}
	if el := elementOf(t, s, "db-dev"); el.At != "a-store_2" || el.Shown != ShownAggregated {
		t.Errorf("db-dev is shown as %+v, want it folded into the second part", el)
	}
	wantWarned(t, s, "view.split /aggregates/0 a-store")
	if !slices.Contains(s.Notes, splitNote) {
		t.Errorf("notes = %q, want the note that says why one name is drawn twice", s.Notes)
	}
}

func TestANameForABoundaryThatNoEvidenceBacksIsLeftOut(t *testing.T) {
	s := scene(t, boundaryCase(t, "two-accounts.json"), &Config{Boundaries: append(slices.Clone(accountNames),
		BoundaryAlias{Member: "mail", Level: "owner", Label: "架空のアカウント"},
		BoundaryAlias{Member: "assets", Level: "network", Label: "架空のVPC"},
	)})
	for _, g := range s.Groups {
		if strings.Contains(g.Label, "架空") {
			t.Errorf("the view config made the boundary %+v", g)
		}
	}
	wantWarned(t, s, "view.boundary_unbacked /boundaries/2 mail", "view.boundary_unbacked /boundaries/3 assets")
}

func TestTheFirstNameOfABoundaryIsKept(t *testing.T) {
	s := scene(t, boundaryCase(t, "two-accounts.json"), &Config{Boundaries: append(slices.Clone(accountNames),
		BoundaryAlias{Member: "db", Level: "owner", Label: "別の名前"},
	)})
	if got := groupOf(t, s, "bd_o_web").Label; got != "本番" {
		t.Errorf("label = %q, want the first name 本番", got)
	}
	wantWarned(t, s, "view.boundary_named_twice /boundaries/2 db")
}

func TestBoundariesDrawnUnderOneNameAreReported(t *testing.T) {
	m := boundaryCase(t, "two-accounts.json")
	wantWarned(t, scene(t, m, &Config{}),
		"view.boundary_label_shared /boundaries web", "view.boundary_label_shared /boundaries web-dev")
	wantWarned(t, scene(t, m, &Config{Boundaries: accountNames[:1]}))
}

func TestAWarningPointsAtTheEntryAsTheFileWritesIt(t *testing.T) {
	m := boundaryCase(t, "two-accounts.json")
	f, diags := ParseConfig("view.yaml", []byte(`schema_version: "0.1"
boundaries:
  - {member: gone, label: なし}
  - {member: web, label: 本番}
  - {member: web-dev, label: 開発}
  - {member: mail, label: 架空}
groups:
  - {id: g-gone, label: なし, members: [gone]}
  - {id: g-data, label: データ, members: [db, db-dev]}
aggregates:
  - {id: a-gone, label: なし, members: [gone]}
  - {id: a-web, label: 受付, members: [web, web-dev]}
`))
	c, _ := fit(t, f, diags, m)
	wantWarned(t, scene(t, m, c),
		"view.boundary_unbacked /boundaries/3 mail", "view.split /groups/1 g-data", "view.split /aggregates/1 a-web")
}

func TestTheSchemasKnowTheNetworkLevel(t *testing.T) {
	network := func(value string) string {
		return modelOf(deployedNode("a", "aws", `,{"key":"scope.network","value":`+value+`}`), "")
	}
	if _, diags := model.Parse("model.json", []byte(network(`"vpc-0aaa1111"`))); len(diags) != 0 {
		t.Errorf("a network written as a string: %v", diags)
	}
	for _, value := range []string{`7`, `""`, `{"kind":"vpc","id":"vpc-0aaa1111"}`} {
		if _, diags := model.Parse("model.json", []byte(network(value))); !diags.HasErrors() {
			t.Errorf("a network written as %s was read", value)
		}
	}
	config := func(level string) string {
		return "schema_version: \"0.1\"\nboundaries:\n  - {member: web, level: " + level + ", label: 本番VPC}\n"
	}
	if _, diags := ParseConfig("view.yaml", []byte(config("network"))); len(diags) != 0 {
		t.Errorf("a name for a network: %v", diags)
	}
	if _, diags := ParseConfig("view.yaml", []byte(config("subnet"))); !diags.HasErrors() {
		t.Error("a name for a level the tool does not have was read")
	}
}

func TestOnlyDirectEvidencePutsANodeInABoundary(t *testing.T) {
	owner := `{"key":"scope.owner","value":{"kind":"account","id":"account-0001"}}`
	weak := func(id, kind string) string {
		return `{"id":"` + id + `","kind":"service","provider":"p","service":"s","evidence":[{"id":"ev-` + id + `","kind":"` + kind + `","supports":[` + owner + `],"ref":{"type":"document","path":"notes.md"},"checked_at":"2026-11-13T00:00:00Z"}]}`
	}
	s := build(t, modelOf(deployedNode("a", "p", ","+owner)+","+weak("b", "stated")+","+weak("c", "hint"), ""), nil)
	wantBoundaries(t, s, map[string]string{"a": "bd_o_a P アカウント", "b": "", "c": ""})
}

func TestARegionAndANetworkOfTheSameNameAreTwoBoundaries(t *testing.T) {
	s := build(t, modelOf(
		deployedNode("a", "p", `,{"key":"region","value":"main"}`)+","+
			deployedNode("b", "p", `,{"key":"region","value":"main"},{"key":"scope.network","value":"main"}`), ""), nil)
	if got := nodeGroups(s); got["a"] != "bd_r_a" || got["b"] != "bd_n_b" {
		t.Errorf("a and b are drawn in %q and %q, want a region and a network", got["a"], got["b"])
	}
}

func TestABoundaryOfHiddenNodesIsNotDrawn(t *testing.T) {
	s := scene(t, boundaryCase(t, "two-accounts.json"), &Config{Boundaries: accountNames, Hidden: []string{"web-dev", "db-dev"}})
	for _, g := range s.Groups {
		if strings.HasSuffix(g.ID, "_web-dev") {
			t.Errorf("the figure draws %+v, whose nodes are all hidden", g)
		}
	}
	wantWarned(t, s)
}

func TestAnAggregateThatSpansLogicalGroupsIsFoldedInEach(t *testing.T) {
	s := scene(t, boundaryCase(t, "two-accounts.json"), &Config{
		Boundaries: accountNames,
		Groups:     []GroupConfig{{ID: "g-web", Label: "受付", Members: []string{"web"}}},
		Aggregates: []AggregateConfig{{ID: "a-prod", Label: "本番の一式", Members: []string{"web", "db"}}},
	})
	first, second := node(t, s, "a-prod"), node(t, s, "a-prod_2")
	if first.GroupID != "g-web" || second.GroupID != "bd_n_web" {
		t.Errorf("parts = %+v and %+v, want one in the group and one beside it, both of a-prod", first, second)
	}
	if first.Role != "1件をまとめて表示" || second.Role != "1件をまとめて表示" {
		t.Errorf("roles = %q and %q, want each part to count its own members", first.Role, second.Role)
	}
	wantWarned(t, s, "view.split /aggregates/0 a-prod")
}

func TestWarningsFollowTheOrderOfTheViewConfig(t *testing.T) {
	s := scene(t, boundaryCase(t, "two-accounts.json"), &Config{
		Boundaries: []BoundaryAlias{{Member: "mail", Label: "架空"}},
		Groups:     []GroupConfig{{ID: "g-data", Label: "データ", Members: []string{"db", "db-dev"}}},
	})
	wantWarned(t, s,
		"view.boundary_unbacked /boundaries/0 mail", "view.split /groups/0 g-data",
		"view.boundary_label_shared /boundaries web", "view.boundary_label_shared /boundaries web-dev")
}
