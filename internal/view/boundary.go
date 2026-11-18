package view

import (
	"slices"

	"github.com/ochamu/ponchimap/internal/assets"
	"github.com/ochamu/ponchimap/internal/model"
)

var ownerKindName = map[string]string{"account": "アカウント", "project": "プロジェクト", "organization": "組織", "team": "チーム"}

// Boundary IDs are generated. The underscore keeps them outside the pattern
// that model IDs and view config IDs must match, so a generated ID can never
// equal a given one.
const boundaryIDPrefix = "bd_"

type levelRule struct {
	level Level
	key   string
	// letter stands for the level in the ID of a boundary, so that the
	// boundaries one node is the first of get different IDs.
	letter string
	// inside is a level that the node has to be in for this one to hold it.
	// A level without one is drawn where the levels outside it leave off: a
	// region with no account stands by itself. A network named by nodes in a
	// region and by nodes of no region would be two boundaries, and it is one
	// network.
	inside Level
	// label reads nothing but the provider and the value of the boundary: a
	// label that counted the other boundaries would move to another boundary
	// when one of them goes.
	label func(provider string, value any) string
}

var levels = []levelRule{
	{LevelOwner, model.KeyScopeOwner, "o", "", func(provider string, value any) string {
		owner, _ := value.(map[string]any)
		kind, _ := owner["kind"].(string)
		return assets.ProviderName(provider) + " " + ownerKindName[kind]
	}},
	{LevelRegion, model.KeyRegion, "r", "", func(_ string, value any) string {
		region, _ := value.(string)
		return "リージョン " + region
	}},
	// The value is a raw identifier, and the level has nothing else to say.
	{LevelNetwork, model.KeyScopeNetwork, "n", LevelRegion, func(string, any) string { return "ネットワーク" }},
}

type boundary struct {
	id, parent string
	level      Level
	// first is the first node of the model that is in the boundary. The ID is
	// made from it, so that the ID stays when nodes are added after it.
	first    string
	label    string
	evidence []string
}

// The view config has no say in realBoundaries, so that hiding, grouping, or
// folding a node renames nothing.
type realBoundaries struct {
	list []boundary
	in   map[string][]Membership
}

func directValues(n *model.Node, key string) (values []any, evidenceIDs []string) {
	seen := map[string]bool{}
	for _, ev := range n.Evidence {
		if !slices.Contains(directKinds, ev.Kind) {
			continue
		}
		for _, s := range ev.Supports {
			if s.Key != key {
				continue
			}
			if c := canonical(s.Value); !seen[c] {
				seen[c] = true
				values = append(values, s.Value)
			}
			if !slices.Contains(evidenceIDs, ev.ID) {
				evidenceIDs = append(evidenceIDs, ev.ID)
			}
		}
	}
	return values, evidenceIDs
}

func deriveBoundaries(m *model.Model) realBoundaries {
	type key struct {
		level                   Level
		parent, provider, value string
	}
	out := realBoundaries{in: map[string][]Membership{}}
	at := map[key]int{}
	for i := range m.Nodes {
		n := &m.Nodes[i]
		outer, contested, placed := "", false, map[Level]bool{}
		for _, rule := range levels {
			values, evidence := directValues(n, rule.key)
			if len(values) == 0 {
				continue
			}
			// Membership is never guessed. Which of the values holds is not
			// for the tool to pick, and neither is which boundary of that
			// level holds the levels inside it: a network drawn for this node
			// outside its contested region would show one network as two.
			var outside Outside
			switch {
			case len(values) > 1:
				outside = OutsideContested
			case contested:
				outside = OutsideOuterContested
			case rule.inside != "" && !placed[rule.inside]:
				outside = OutsideOuterUnstated
			}
			if outside != "" {
				contested = contested || outside != OutsideOuterUnstated
				out.in[n.ID] = append(out.in[n.ID], Membership{Level: rule.level, Outside: outside})
				continue
			}
			k := key{rule.level, outer, n.Provider, canonical(values[0])}
			if _, ok := at[k]; !ok {
				at[k] = len(out.list)
				out.list = append(out.list, boundary{
					id: boundaryIDPrefix + rule.letter + "_" + n.ID, parent: outer, level: rule.level, first: n.ID,
					label: rule.label(n.Provider, values[0]),
				})
			}
			bd := &out.list[at[k]]
			bd.evidence = append(bd.evidence, evidence...)
			out.in[n.ID] = append(out.in[n.ID], Membership{Level: rule.level, Boundary: bd.id})
			outer, placed[rule.level] = bd.id, true
		}
	}
	return out
}
