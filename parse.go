// Copyright 2018 Joshua J Baker. All rights reserved.

package osmfile

import (
	"errors"
	"fmt"
	"io"
	"unsafe"

	"github.com/tidwall/osmfile/internal/pbf"
)

// What do you want to parse?
type What int

// What options
const (
	Everything What = iota
	DataKinds       // for only detecting block data kinds
	Strings         // for processing all strings
	Nodes           // for process all nodes
	Ways            // for processing all ways
	Relations       // for processing all relations
)

func procBlock(what What, data []byte) (Block, error) {
	block := Block{
		granularity:     100,
		latOffset:       0,
		lonOffset:       0,
		dateGranularity: 1000,
	}
	var stringTable []byte
	var primativeGroups [][]byte
	err := pbf.ForEachField(data, func(f pbf.Field) error {
		switch f.Num() {
		case 1:
			stringTable = f.Data()
		case 2:
			if what == DataKinds {
				var perr error
				block.dataKind, perr =
					onlyDetectPrimativeDataKind(what, f.Data())
				if perr != nil {
					return perr
				}
			} else if what != Strings {
				primativeGroups = append(primativeGroups, f.Data())
			}
		case 17:
			block.granularity = f.Int64()
		case 18:
			block.dateGranularity = f.Int64()
		case 19:
			block.latOffset = f.Int64()
		case 20:
			block.lonOffset = f.Int64()
		default:
			return fmt.Errorf("unsupported field: %d", f.Num())
		}
		return nil
	})
	if err != nil {
		return Block{}, err
	}
	if what != DataKinds {
		if err := procStringTable(what, stringTable, &block); err != nil {
			return Block{}, err
		}
		for _, primativeGroup := range primativeGroups {
			err := procPrimativeGroup(what, primativeGroup, &block)
			if err != nil {
				return Block{}, err
			}
		}
	}
	return block, nil
}

func onlyDetectPrimativeDataKind(what What, data []byte) (int, error) {
	dataKind := -1
	err := pbf.ForEachField(data, func(f pbf.Field) error {
		switch f.Num() {
		case 2:
			dataKind = 0
			return io.EOF
		case 3:
			dataKind = 1
			return io.EOF
		case 4:
			dataKind = 2
			return io.EOF
		}
		return nil
	})
	if err == io.EOF {
		err = nil
	}
	return dataKind, err
}

func procStringTable(what What, data []byte, block *Block) error {
	var count int  // number of string
	var length int // total length of all strings
	if err := pbf.ForEachField(data, func(f pbf.Field) error {
		count++
		length += len(f.Data())
		return nil
	}); err != nil {
		return err
	}
	block.stringsCount = count
	stringsOneBytes := make([]byte, 0, length)
	block.strings = make([]string, 0, length)

	// binary.LittleEndian.PutUint64(strmap[0:], uint64(count))
	// posidx := 0
	// posstr := posidx + count*8
	pbf.ForEachField(data, func(f pbf.Field) error {
		mark := len(stringsOneBytes)
		stringsOneBytes = append(stringsOneBytes, f.Data()...)
		bytes := stringsOneBytes[mark:]
		str := *(*string)(unsafe.Pointer(&bytes))
		block.strings = append(block.strings, str)
		return nil
	})
	block.stringsOne = *(*string)(unsafe.Pointer(&stringsOneBytes))
	return nil
}

func procPrimativeGroup(what What, data []byte, block *Block) error {
	return pbf.ForEachField(data, func(f pbf.Field) error {
		switch f.Num() {
		case 1:
			return errors.New("plain node pbf type not supported")
		case 2:
			block.dataKind = 0
			if what == Everything || what == Nodes {
				procDenseNodes(what, f.Data(), block)
			}
		case 3:
			block.dataKind = 1
			if what == Everything || what == Ways {
				procWay(what, f.Data(), block)
			}
		case 4:
			block.dataKind = 2
			if what == Everything || what == Relations {
				procRelation(what, f.Data(), block)
			}
		case 5:
			// ignore changeset
		default:
			return fmt.Errorf("unsupported primative group field: %d", f.Num())
		}
		return nil
	})
}

func procDenseNodes(what What, data []byte, block *Block) error {
	// count the number of nodes and the strings
	var numNodes int
	var numStrings int
	err := pbf.ForEachField(data, func(f pbf.Field) error {
		var err error
		switch f.Num() {
		case 1:
			return f.ForEachPackedInt64(func(x int64) error {
				numNodes++
				return nil
			})
		case 10:
			var which bool
			err = f.ForEachPackedUint64(func(x uint64) error {
				if !which && x == 0 {
					return nil
				}
				numStrings++
				which = !which
				return nil
			})
		}
		return err
	})
	if err != nil {
		return err
	}
	nodes := make([]blockNode, numNodes)
	nodeStrings := make([]uint32, numStrings)
	var idAdder int64
	var latAdder int64
	var lonAdder int64

	err = pbf.ForEachField(data, func(f pbf.Field) error {
		var err error
		var i int
		switch f.Num() {
		case 1:
			err = f.ForEachPackedInt64(func(x int64) error {
				idAdder += x
				nodes[i].id = idAdder
				i++
				return nil
			})
		case 8:
			err = f.ForEachPackedInt64(func(x int64) error {
				latAdder += x
				nodes[i].lat = .000000001 * (float64)(block.latOffset+
					(block.granularity*latAdder))
				i++
				return nil
			})
		case 9:
			err = f.ForEachPackedInt64(func(x int64) error {
				lonAdder += x
				nodes[i].lon = .000000001 * (float64)(block.lonOffset+
					(block.granularity*lonAdder))
				i++
				return nil
			})
		case 10:
			var stringIdx uint32
			var nodeIdx int
			var which bool
			var first bool
			err = f.ForEachPackedUint64(func(x uint64) error {
				if !which && x == 0 {
					nodeIdx++
					first = false
					return nil
				}
				if !first {
					nodes[nodeIdx].sset = stringIdx
					first = true
				}
				nodeStrings[stringIdx] = uint32(x)
				stringIdx++
				nodes[nodeIdx].send = stringIdx
				which = !which
				return nil
			})
		}
		return err
	})
	if err != nil {
		return err
	}
	block.nodes = append(block.nodes, nodes...)
	block.nodeStrings = append(block.nodeStrings, nodeStrings...)
	return nil
}

func procWay(what What, data []byte, block *Block) error {
	//
	// message Way {
	// 	required int64 id = 1;
	// 	// Parallel arrays.
	// 	repeated uint32 keys = 2 [packed = true];
	// 	repeated uint32 vals = 3 [packed = true];
	//
	// 	optional Info info = 4;
	//
	// 	repeated sint64 refs = 8 [packed = true];  // DELTA coded
	// }
	//

	var way blockWay
	way.sset = uint32(len(block.wayStrings))
	way.rset = uint32(len(block.wayRefs))
	strValIdx := len(block.wayStrings) + 1
	err := pbf.ForEachField(data, func(f pbf.Field) error {
		switch f.Num() {
		case 1:
			way.id = int64(f.Uint64())
		case 2:
			err := f.ForEachPackedUint64(func(x uint64) error {
				block.wayStrings = append(block.wayStrings, uint32(x), 0)
				return nil
			})
			if err != nil {
				return err
			}
		case 3:
			err := f.ForEachPackedUint64(func(x uint64) error {
				block.wayStrings[strValIdx] = uint32(x)
				strValIdx += 2
				return nil
			})
			if err != nil {
				return err
			}
		case 8:
			var refAdder int64
			err := f.ForEachPackedInt64(func(x int64) error {
				refAdder += x
				block.wayRefs = append(block.wayRefs, refAdder)
				return nil
			})
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	way.send = uint32(len(block.wayStrings))
	way.rend = uint32(len(block.wayRefs))
	block.ways = append(block.ways, way)
	return nil
}

func procRelation(what What, data []byte, block *Block) error {
	//
	// message Relation {
	// 	enum MemberType {
	// 	  NODE = 0;
	// 	  WAY = 1;
	// 	  RELATION = 2;
	// 	}
	// 	 required int64 id = 1;
	//
	// 	 // Parallel arrays.
	// 	 repeated uint32 keys = 2 [packed = true];
	// 	 repeated uint32 vals = 3 [packed = true];
	//
	// 	 optional Info info = 4;
	//
	// 	 // Parallel arrays
	// 	 repeated int32 roles_sid = 8 [packed = true];
	// 	 repeated sint64 memids = 9 [packed = true]; // DELTA encoded
	// 	 repeated MemberType types = 10 [packed = true];
	// }
	//
	var relation blockRelation
	relation.sset = uint32(len(block.relationStrings))
	relation.mset = uint32(len(block.relationMemberRefs))
	strValIdx := len(block.relationStrings) + 1
	err := pbf.ForEachField(data, func(f pbf.Field) error {
		switch f.Num() {
		case 1:
			relation.id = int64(f.Uint64())
		case 2:
			err := f.ForEachPackedUint64(func(x uint64) error {
				block.relationStrings = append(block.relationStrings,
					uint32(x), 0)
				return nil
			})
			if err != nil {
				return err
			}
		case 3:
			err := f.ForEachPackedUint64(func(x uint64) error {
				block.relationStrings[strValIdx] = uint32(x)
				strValIdx += 2
				return nil
			})
			if err != nil {
				return err
			}
		case 8:
			err := f.ForEachPackedUint64(func(x uint64) error {
				block.relationMemberRoles = append(block.relationMemberRoles,
					uint32(x))
				return nil
			})
			if err != nil {
				return err
			}
		case 9:
			var delta int64
			err := f.ForEachPackedInt64(func(x int64) error {
				delta += x
				block.relationMemberRefs = append(block.relationMemberRefs,
					delta)
				return nil
			})
			if err != nil {
				return err
			}
		case 10:
			err := f.ForEachPackedUint64(func(x uint64) error {
				block.relationMemberTypes = append(block.relationMemberTypes,
					byte(x))
				return nil
			})
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	relation.send = uint32(len(block.relationStrings))
	relation.mend = uint32(len(block.relationMemberRefs))
	block.relations = append(block.relations, relation)
	return nil
}
