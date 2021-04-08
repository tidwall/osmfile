// Copyright 2018 Joshua J Baker. All rights reserved.

package osmfile

type DataKind int

const (
	DataKindNodes     DataKind = 0
	DataKindWays      DataKind = 1
	DataKindRelations DataKind = 2
)

func (k DataKind) String() string {
	switch k {
	case DataKindNodes:
		return "nodes"
	case DataKindWays:
		return "ways"
	case DataKindRelations:
		return "relations"
	default:
		return "unknown"
	}
}

type blockNode struct {
	id   int64
	lat  float64
	lon  float64
	sset uint32 // position of first string
	send uint32 // position of last string plus one
}

// Node ...
type Node struct {
	blockNode
	block Block
}

// ID ...
func (n Node) ID() int64 {
	return n.id
}

// Lat ...
func (n Node) Lat() float64 {
	return n.lat
}

// Lon ...
func (n Node) Lon() float64 {
	return n.lon
}

// NumStrings ...
func (n Node) NumStrings() int {
	return int(n.send - n.sset)
}

// StringAt ...
func (n Node) StringAt(index int) string {
	return n.block.StringAt(int(n.block.nodeStrings[n.sset:n.send][index]))
}

type blockRelation struct {
	id   int64
	sset uint32 // position of first string
	send uint32 // position of last string plus one
	mset uint32 // position of first member ref
	mend uint32 // position of last member ref plus one
}

// Relation ..
type Relation struct {
	blockRelation
	block Block
}

// ID ...
func (r Relation) ID() int64 {
	return r.id
}

// NumStrings ...
func (r Relation) NumStrings() int {
	return int(r.send - r.sset)
}

// StringAt ...
func (r Relation) StringAt(index int) string {
	return r.block.StringAt(int(r.block.relationStrings[r.sset:r.send][index]))
}

// NumMembers ...
func (r Relation) NumMembers() int {
	return int(r.mend - r.mset)
}

// MemberAt ...
func (r Relation) MemberAt(index int) (typ byte, ref int64, role string) {
	typ = r.block.relationMemberTypes[r.mset:r.mend][index]
	ref = r.block.relationMemberRefs[r.mset:r.mend][index]
	role = r.block.StringAt(int(
		r.block.relationMemberRoles[r.mset:r.mend][index],
	))
	return
}

type blockWay struct {
	id   int64
	sset uint32 // position of first string
	send uint32 // position of last string plus one
	rset uint32 // position of first ref
	rend uint32 // position of last ref
}

// Way ...
type Way struct {
	blockWay
	block Block
}

// ID ...
func (w Way) ID() int64 {
	return w.id
}

// NumRefs ...
func (w Way) NumRefs() int {
	return int(w.rend - w.rset)
}

// RefAt ...
func (w Way) RefAt(index int) int64 {
	return w.block.wayRefs[w.rset:w.rend][index]
}

// NumStrings ...
func (w Way) NumStrings() int {
	return int(w.send - w.sset)
}

// StringAt ...
func (w Way) StringAt(index int) string {
	return w.block.StringAt(int(w.block.wayStrings[w.sset:w.send][index]))
}

// Block ...
type Block struct {
	// skip            bool
	granularity     int64
	latOffset       int64
	lonOffset       int64
	dateGranularity int64
	// shared
	// num          int
	dataKind     int // 0 = nodes, 1 = ways, 2 = relations
	stringsCount int
	stringsOne   string
	strings      []string
	// nodes
	nodes       []blockNode
	nodeStrings []uint32
	// ways
	ways       []blockWay
	wayStrings []uint32
	wayRefs    []int64
	// relations
	relations           []blockRelation
	relationStrings     []uint32
	relationMemberRoles []uint32
	relationMemberRefs  []int64
	relationMemberTypes []byte
}

// // Weight ...
// func (b Block) Weight() uint64 {
// 	return uint64(0 +
// 		int(unsafe.Sizeof(Block{})) +
// 		len(b.stringsOne) + len(b.strings)*int(unsafe.Sizeof("")) +
// 		cap(b.nodes)*int(unsafe.Sizeof(blockNode{})) + cap(b.nodeStrings)*4 +
// 		cap(b.ways)*int(unsafe.Sizeof(blockWay{})) + cap(b.wayStrings)*4 +
// 		/* */ cap(b.wayRefs)*8 +
// 		cap(b.relations)*int(unsafe.Sizeof(blockRelation{})) +
// 		/* */ cap(b.relationStrings)*4 +
// 		0,
// 	)
// }

// DataKind ...
func (b Block) DataKind() DataKind {
	return DataKind(b.dataKind)
}

// // Index ...
// func (b Block) Index() int {
// 	return b.num
// }

// NumStrings ...
func (b Block) NumStrings() int {
	return b.stringsCount
}

// StringAt ...
func (b Block) StringAt(index int) string {
	return b.strings[index]
}

// NumNodes ...
func (b Block) NumNodes() int {
	return len(b.nodes)
}

// NodeAt ...
func (b Block) NodeAt(index int) Node {
	return Node{blockNode: b.nodes[index], block: b}
}

// NumWays ...
func (b Block) NumWays() int {
	return len(b.ways)
}

// WayAt ...
func (b Block) WayAt(index int) Way {
	return Way{blockWay: b.ways[index], block: b}
}

// NumRelations ...
func (b Block) NumRelations() int {
	return len(b.relations)
}

// RelationAt ...
func (b Block) RelationAt(index int) Relation {
	return Relation{blockRelation: b.relations[index], block: b}
}
