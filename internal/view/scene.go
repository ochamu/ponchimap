// Package view derives the display model (Scene) from the configuration model
// and the view config. A Scene is recomputed on every render and is never a
// second source of truth.
package view

import (
	"fmt"

	"github.com/ochamu/ponchimap/internal/diag"
)

// Scene is what a Renderer draws. Scene IDs are model IDs and view config
// IDs, or are made from them. One ID names one element, and a group's
// parent always precedes it in Groups.
type Scene struct {
	Title     string
	Direction Direction
	Groups    []Group
	Nodes     []Node
	Edges     []Edge
	Legend    []LegendItem
	Notes     []string
	// Elements says how the figure shows each node and edge of the model,
	// nodes first, in model order.
	Elements []Element
	// Assessment is the state of every element of the model, hidden ones
	// too.
	Assessment *Assessment
	// Warnings are what Build drew otherwise than the view config asked,
	// or what a reader of the figure may trip over. None has a file, line,
	// or column. A warning about an entry of the view config names it by its
	// JSON Pointer. A warning about a node of the model alone has no pointer
	// and names the node as its target; these come first, in model order.
	Warnings diag.List
}

// Element says how the figure shows one node or edge of the model.
type Element struct {
	ID string
	// At is the scene node or edge that shows it, and is empty when it is
	// not drawn.
	At    string
	Shown Shown
}

// Level is a kind of real boundary. A node sits in at most one boundary of
// each level, and the levels nest in this order.
type Level string

const (
	LevelOwner   Level = "owner"
	LevelRegion  Level = "region"
	LevelNetwork Level = "network"
)

type Membership struct {
	Level Level
	// Boundary is the ID of the boundary. It is empty when the node is in no
	// boundary of the level, and Outside says why.
	Boundary string
	Outside  Outside
}

// Outside is why a node is in no boundary of a level that its evidence
// states. The boundaries of a level lie inside those of the levels outside
// it, so a node is placed in one only where that leaves no doubt about which.
type Outside string

const (
	// OutsideContested: the evidence gives more than one value for the level.
	OutsideContested Outside = "contested"
	// OutsideOuterContested: a level outside this one is contested.
	OutsideOuterContested Outside = "outer_contested"
	// OutsideOuterUnstated: the level is drawn inside another that the
	// evidence does not state, as a network is inside a region.
	OutsideOuterUnstated Outside = "outer_unstated"
)

type Shown string

const (
	// ShownCard is a node drawn as one card.
	ShownCard Shown = "card"
	// ShownAggregated is a node folded into the aggregate At.
	ShownAggregated Shown = "aggregated"
	// ShownLine is an edge drawn as the line At, alone or with other edges.
	ShownLine Shown = "line"
	// ShownFolded is an edge whose ends are both inside the aggregate At.
	ShownFolded Shown = "folded"
	// ShownSelf is an edge from a node to itself. Its state is a mark on the
	// node At.
	ShownSelf Shown = "self"
	// ShownHidden is an element the view config hides.
	ShownHidden Shown = "hidden"
	// ShownEndHidden is an edge with an end the view config hides.
	ShownEndHidden Shown = "end_hidden"
)

type Direction string

const (
	DirRight Direction = "right"
	DirDown  Direction = "down"
)

type GroupKind string

const (
	// GroupBoundary is a real boundary backed by scope evidence.
	GroupBoundary GroupKind = "boundary"
	// GroupLogical exists only because the view config asked for it.
	GroupLogical GroupKind = "logical"
)

type Group struct {
	ID       string
	ParentID string
	Kind     GroupKind
	Label    string
	// Level is the level of a real boundary. A logical group has none.
	Level Level
	// EvidenceIDs state the membership of the nodes of a real boundary, the
	// nodes the view config hides too, in model order.
	EvidenceIDs []string
}

type Frame string

const (
	FrameSolid  Frame = "solid"
	FrameDashed Frame = "dashed"
)

type MarkKind string

const (
	MarkUnverified MarkKind = "unverified"
	MarkMismatch   MarkKind = "mismatch"
)

func (k MarkKind) Label() string {
	if k == MarkMismatch {
		return "不一致"
	}
	return "未照合"
}

// MarkSubject says what carries the state a mark announces.
type MarkSubject string

const (
	// MarkOfElement is the node itself, or the members of an aggregate.
	MarkOfElement MarkSubject = ""
	// MarkOfRelation is the relations whose ends both fall in the node. They
	// have no line of their own, so the node is the only place their state
	// shows.
	MarkOfRelation MarkSubject = "relation"
)

type Mark struct {
	Kind MarkKind
	Of   MarkSubject
	// Count is the number of source elements carrying the mark. It exceeds 1
	// only on aggregated nodes.
	Count int
}

// Text is what a mark says when drawn. A mark about folded relations reads
// differently from one about the element, so the two are never added up.
func (m Mark) Text() string {
	text := m.Kind.Label()
	if m.Of == MarkOfRelation {
		text = "関係の" + text
	}
	if m.Count > 1 {
		text += fmt.Sprintf(" %d件", m.Count)
	}
	return text
}

type Node struct {
	ID        string
	SourceIDs []string
	GroupID   string
	Title     string
	Role      string
	IconKey   string
	Frame     Frame
	Marks     []Mark
	// FoldedEdgeIDs are the model edges whose ends both fall in this node:
	// edges inside an aggregate, and edges from a node to itself. They are
	// not drawn, and stay here so that the figure can still be traced back to
	// them. Any unverified or mismatch state they carry is in Marks, under
	// MarkOfRelation.
	FoldedEdgeIDs []string
}

type Line string

const (
	LineSolid  Line = "solid"
	LineDashed Line = "dashed"
)

type Edge struct {
	ID        string
	SourceIDs []string
	From      string
	To        string
	Label     string
	Line      Line
	// State is the state of the edges the line shows, mismatch over
	// unverified, and empty when they have none.
	State MarkKind
}

type LegendItem string

const (
	LegendSolidLine   LegendItem = "solid-line"
	LegendDashedLine  LegendItem = "dashed-line"
	LegendSolidFrame  LegendItem = "solid-frame"
	LegendDashedFrame LegendItem = "dashed-frame"
	LegendUnverified  LegendItem = "unverified"
	LegendMismatch    LegendItem = "mismatch"
	LegendBoundary    LegendItem = "boundary"
	LegendLogical     LegendItem = "logical"
)

var LegendOrder = []LegendItem{
	LegendSolidLine, LegendDashedLine, LegendSolidFrame, LegendDashedFrame,
	LegendUnverified, LegendMismatch, LegendBoundary, LegendLogical,
}

var legendText = map[LegendItem]string{
	LegendSolidLine:   "根拠のある関係（設定・実装・観測）",
	LegendDashedLine:  "手がかり・説明のみの関係",
	LegendSolidFrame:  "宣言・実体・利用実装のある要素",
	LegendDashedFrame: "手がかり・説明のみの要素",
	LegendUnverified:  "実環境と未照合",
	LegendMismatch:    "根拠どうしの食い違い",
	LegendBoundary:    "実在する境界（アカウント等）",
	LegendLogical:     "説明のための論理グループ",
}

func (i LegendItem) Text() string { return legendText[i] }

// LegendNote is always shown: a solid line is backed by evidence but is not a
// claim about current uptime or traffic.
const LegendNote = "実線は根拠のある関係を示します。現在の稼働や通信実績を保証するものではありません。"
