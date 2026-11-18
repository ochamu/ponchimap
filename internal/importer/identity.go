package importer

import (
	"cmp"
	"encoding/json"
	"maps"
	"slices"
)

type description struct {
	kind, provider, service        string
	environment                    string
	native, name, account, project string
}

func describe(kind, provider, service, environment string, identity map[string]any) description {
	return description{kind: kind, provider: provider, service: service, environment: environment,
		native: identityText(identity["native_id"]), name: identityText(identity["logical_name"]),
		account: identityText(identity["account_id"]), project: identityText(identity["project_id"])}
}

// A number in a model written by hand is compared by its digits.
func identityText(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case json.Number:
		return x.String()
	}
	return ""
}

// tier is a tier of the main key. The values stand in the order of the tiers,
// which compareKeys and N3 compare.
type tier int

const (
	tierOwnerAndNative tier = iota + 1
	tierNativeInEnvironment
	tierNameInEnvironment
	tierService
)

// key is a main key of Identity. A field that the tier does not read is
// "", and so is an owner that is absent, which is a value of its own.
type key struct {
	tier                    tier
	kind, provider, service string
	environment             string
	account, project        string
	name                    string
}

// A resource of a model written by hand that has neither a native_id nor a
// logical_name has no main key, and is the same as nothing.
func (d description) key() (key, bool) {
	k := key{kind: d.kind, provider: d.provider, service: d.service}
	switch {
	case d.native != "" && (d.account != "" || d.project != ""):
		k.tier, k.account, k.project, k.name = tierOwnerAndNative, d.account, d.project, d.native
	case d.native != "":
		k.tier, k.environment, k.name = tierNativeInEnvironment, d.environment, d.native
	case d.name != "":
		k.tier, k.environment, k.account, k.project, k.name = tierNameInEnvironment, d.environment, d.account, d.project, d.name
	case d.kind == "service":
		k = key{tier: tierService, provider: d.provider, service: d.service}
	default:
		return key{}, false
	}
	return k, true
}

// fields are the values of the key in the order of its tier in Identity,
// which is what the ID of a new node is made from.
func (k key) fields() []string {
	switch k.tier {
	case tierOwnerAndNative:
		return []string{k.kind, k.provider, k.service, k.account, k.project, k.name}
	case tierNativeInEnvironment:
		return []string{k.kind, k.provider, k.service, k.environment, k.name}
	case tierNameInEnvironment:
		return []string{k.kind, k.provider, k.service, k.environment, k.account, k.project, k.name}
	}
	return []string{k.provider, k.service}
}

func compareKeys(a, b key) int {
	return cmp.Or(cmp.Compare(a.tier, b.tier), slices.Compare(a.fields(), b.fields()))
}

// where says in which environments and under which names a node is known.
type where struct{ environments, names map[string]bool }

func newWhere() where { return where{map[string]bool{}, map[string]bool{}} }

func (w where) add(d description) {
	w.environments[d.environment] = true
	if d.name != "" {
		w.names[d.name] = true
	}
}

func (w where) take(other where) {
	maps.Copy(w.environments, other.environments)
	maps.Copy(w.names, other.names)
}

// group is the descriptions with one main key, which N1 makes one node.
type group struct {
	key                  key
	existing, candidates []int
	// now is where the descriptions that state the key in this run know it:
	// the candidates, and what the acquisitions that stay say. once is where
	// the identity of a node of the model knows it, which no acquisition may
	// say any more.
	now, once where
	joined    *group
	// held: the records say which node the descriptions of this key are, and
	// their evidence stands on that node. No rule decides it again.
	held      bool
	ambiguous bool
}

func (g *group) root() *group {
	for g.joined != nil {
		g = g.joined
	}
	return g
}

type nodeSet struct {
	// Import does not merge the existing nodes of a set (N5).
	existing    []int
	candidates  []int
	modelNodeOf map[int]int
	key         key
}

// known is a node of the model as the matching sees it. The model keeps one
// identity and one environment of a node, which is less than the descriptions
// that made it: so the records keep those, and the matching reads them.
type known struct {
	// own is what the identity and the environment of the node say. It gives
	// the main key of the node.
	own description
	// byItself: the node has evidence that no record accounts for, such as
	// evidence written by hand, so own states the node as an acquisition
	// would.
	byItself bool
	// stated is what the acquisitions that stay in the model, and are not
	// given again in this run, say of the node.
	stated []description
}

// match applies N1 to N5 to the nodes of the model and the candidates of one
// run.
//
// left are the nodes of the model that leave it in this run. They take no
// part: the next run, which no longer finds them, would otherwise match the
// candidates in another way.
//
// before is the node that the descriptions of a key were taken as when they
// were last read.
func match(existing []known, candidates []description, left map[int]bool, before map[key]int) (sets []nodeSet, ambiguous []int) {
	var groups []*group
	byKey := map[key]*group{}
	place := func(d description) *group {
		k, ok := d.key()
		if !ok {
			return nil
		}
		g := byKey[k]
		if g == nil {
			g = &group{key: k, now: newWhere(), once: newWhere()}
			byKey[k] = g
			groups = append(groups, g)
		}
		return g
	}
	// home is the group of the main key of a node of the model.
	home := map[int]*group{}
	for i, n := range existing {
		if left[i] {
			continue
		}
		own := place(n.own)
		switch {
		case own == nil:
		case n.byItself:
			own.now.add(n.own)
		default:
			own.once.add(n.own)
		}
		for _, d := range n.stated {
			g := place(d)
			if g == nil {
				continue
			}
			g.now.add(d)
			switch {
			case g == own:
				continue
			case own == nil:
				own = g
				continue
			case g.joined == nil && len(g.existing) == 0:
				g.joined, g.held = own, true
			}
			if !slices.Contains(g.existing, i) {
				g.existing = append(g.existing, i)
			}
		}
		if own != nil {
			own.existing = append(own.existing, i)
			home[i] = own
		}
	}
	for i, d := range candidates {
		// A candidate always has a key: the schema of the findings refuses a
		// resource without a native_id and a logical_name.
		if g := place(d); g != nil {
			g.now.add(d)
			g.candidates = append(g.candidates, i)
		}
	}

	// join settles which of the nodes that fit a group it is. The nodes that
	// are stated now come first: the identity of a node counts only when no
	// such node fits. With one fit the group is that node. With more it stays
	// with the node that it was taken as before, and when that node is not
	// among them, it stays on its own (N4).
	join := func(g *group, fitting func(now bool) []*group) {
		fits := fitting(true)
		if len(fits) == 0 {
			fits = fitting(false)
		}
		kept, remembered := before[g.key]
		switch {
		case len(fits) == 1:
			g.joined = fits[0]
		case len(fits) > 1 && remembered && home[kept] != nil && slices.Contains(fits, home[kept].root()):
			g.joined = home[kept].root()
		case len(fits) > 1:
			g.ambiguous = true
		}
	}
	sameService := func(a, b *group) bool {
		return a.key.kind == b.key.kind && a.key.provider == b.key.provider && a.key.service == b.key.service
	}
	// N2.
	for _, g := range groups {
		if g.key.tier != tierNativeInEnvironment || g.held {
			continue
		}
		join(g, func(now bool) (fits []*group) {
			for _, other := range groups {
				in := other.now.environments[g.key.environment] || (!now && other.once.environments[g.key.environment])
				if other.key.tier == tierOwnerAndNative && sameService(g, other) && other.key.name == g.key.name && in {
					fits = append(fits, other)
				}
			}
			return fits
		})
	}
	// N3. A description with a name joins a node only when the node carries
	// every owner that the description writes: joining on a name alone would
	// put the same name of another account into the node. So a join adds no
	// owner to a node, and the key of a node is the key of its root.
	//
	// The descriptions that write more of the owner are settled first, and a
	// description counts the nodes as the ones before it left them. Two
	// descriptions that write equally much never fit each other, so the order
	// among them cannot matter.
	//
	// A description with a name that the records hold on a node adds neither
	// an environment nor a name to it: what lets a name join a node is what
	// the node is known by apart from the names that joined it.
	type node struct{ now, once where }
	nodes := map[*group]*node{}
	for _, g := range groups {
		if g.held && g.key.tier == tierNameInEnvironment {
			continue
		}
		n := nodes[g.root()]
		if n == nil {
			n = &node{newWhere(), newWhere()}
			nodes[g.root()] = n
		}
		n.now.take(g.now)
		n.once.take(g.once)
	}
	owners := func(g *group) int {
		count := 0
		for _, owner := range []string{g.key.account, g.key.project} {
			if owner != "" {
				count++
			}
		}
		return count
	}
	var named []*group
	for _, g := range groups {
		if g.key.tier == tierNameInEnvironment && !g.held {
			named = append(named, g)
		}
	}
	slices.SortStableFunc(named, func(a, b *group) int { return cmp.Compare(owners(b), owners(a)) })
	for _, g := range named {
		join(g, func(now bool) (fits []*group) {
			for _, other := range groups {
				n := nodes[other]
				if other == g || n == nil {
					continue
				}
				// Such a node knows more than the description: it has another
				// key, so it has a native ID or an owner that the description
				// does not write.
				carries := (g.key.account == "" || g.key.account == other.key.account) &&
					(g.key.project == "" || g.key.project == other.key.project)
				in := n.now.environments[g.key.environment] || (!now && n.once.environments[g.key.environment])
				called := n.now.names[g.key.name] || (!now && n.once.names[g.key.name])
				if sameService(g, other) && in && called && carries {
					fits = append(fits, other)
				}
			}
			return fits
		})
		if g.joined != nil {
			delete(nodes, g)
		}
	}

	members := map[*group][]*group{}
	for _, g := range groups {
		members[g.root()] = append(members[g.root()], g)
		if g.ambiguous {
			ambiguous = append(ambiguous, g.candidates...)
		}
	}
	for _, g := range groups {
		var s nodeSet
		for i, member := range members[g] {
			s.existing = append(s.existing, member.existing...)
			s.candidates = append(s.candidates, member.candidates...)
			if i == 0 || compareKeys(member.key, s.key) < 0 {
				s.key = member.key
			}
		}
		if len(s.candidates) > 0 {
			// A node stands in the group of each key that it answers to.
			slices.Sort(s.existing)
			s.existing = slices.Compact(s.existing)
			slices.Sort(s.candidates)
			if len(s.existing) > 0 {
				s.modelNodeOf = map[int]int{}
				for _, member := range members[g] {
					to, remembered := before[member.key]
					if !remembered || !slices.Contains(s.existing, to) {
						to = s.existing[0]
						for at := member; at != nil; at = at.joined {
							if len(at.existing) > 0 {
								to = at.existing[0]
								break
							}
						}
					}
					for _, c := range member.candidates {
						s.modelNodeOf[c] = to
					}
				}
			}
			sets = append(sets, s)
		}
	}
	slices.Sort(ambiguous)
	return sets, ambiguous
}
