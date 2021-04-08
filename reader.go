// Copyright 2021 Joshua J Baker. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package osmfile

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"errors"
	"io"
	"io/ioutil"

	"github.com/tidwall/osmfile/internal/pbf"
)

type rawBlockReader struct {
	r   io.Reader
	err error
	pos int64
}

func newRawBlockReader(r io.Reader) *rawBlockReader {
	return &rawBlockReader{r: r}
}

type rawBlock struct {
	Type string
	Data []byte
}

func (r *rawBlockReader) ReadBlock() (n int, block rawBlock, err error) {
	if r.err != nil {
		return 0, rawBlock{}, r.err
	}
	bpos := r.pos
	var buf [4]byte
	if _, err := io.ReadFull(r.r, buf[:]); err != nil {
		r.err = err
		return 0, rawBlock{}, r.err
	}
	r.pos += 4

	hdrLen := binary.BigEndian.Uint32(buf[:])
	hdr := make([]byte, hdrLen)
	if _, err := io.ReadFull(r.r, hdr); err != nil {
		r.err = err
		return 0, rawBlock{}, r.err
	}
	r.pos += int64(hdrLen)
	/*
		message BlobHeader {
			required string type = 1;
			optional bytes indexdata = 2;
			required int32 datasize = 3;
		}
	*/
	var btype string
	var bsize int
	err = pbf.ForEachField(hdr, func(f pbf.Field) error {
		switch f.Num() {
		case 1:
			btype = string(f.Data())
		case 3:
			bsize = int(f.Uint64())
		}
		return nil
	})
	if err != nil {
		r.err = err
		return 0, rawBlock{}, err
	}

	bdata := make([]byte, bsize)
	if _, err := io.ReadFull(r.r, bdata); err != nil {
		r.err = err
		return 0, rawBlock{}, r.err
	}
	r.pos += int64(bsize)
	return int(r.pos - bpos), rawBlock{Type: btype, Data: bdata}, nil
}

// BlockReader is a reader for reading OSMData blocks from an OSM Planet
// protobuf file.
type BlockReader struct {
	rr *rawBlockReader
}

// NewBlockReader returns a reader for reading OSMData blocks from an OSM Planet
// protobuf file.
func NewBlockReader(r io.Reader) *BlockReader {
	return &BlockReader{rr: newRawBlockReader(r)}
}

// ReadBlock reads the next OSMData block.
// Returns the number of bytes read and the block.
func (r *BlockReader) ReadBlock() (n int, block Block, err error) {
	for {
		nn, rblock, err := r.rr.ReadBlock()
		if err != nil {
			return 0, Block{}, err
		}
		n += nn
		if rblock.Type != "OSMData" {
			continue
		}
		data, err := inflate(rblock.Data)
		if err != nil {
			return 0, Block{}, err
		}
		block, err := procBlock(Everything, data)
		if err != nil {
			return 0, Block{}, err
		}
		return n, block, nil
	}
}

// SkipBlock skips over the next block. Like ReadBlock but faster.
func (r *BlockReader) SkipBlock() (n int, err error) {
	for {
		nn, rblock, err := r.rr.ReadBlock()
		if err != nil {
			return 0, err
		}
		n += nn
		if rblock.Type != "OSMData" {
			continue
		}
		return n, nil
	}
}

func inflate(bdata []byte) (data []byte, err error) {
	/*
	   message Blob {
	       optional bytes raw = 1; // No compression
	       optional int32 raw_size = 2; // When compressed, the uncompressed size

	       // Possible compressed versions of the data.
	       optional bytes zlib_data = 3;

	       // PROPOSED feature for LZMA compressed data. SUPPORT IS NOT REQUIRED.
	       optional bytes lzma_data = 4;

	       // Formerly used for bzip2 compressed data. Depreciated in 2010.
	       optional bytes OBSOLETE_bzip2_data = 5 [deprecated=true]; // Don't ...
	       // ... reuse this tag number.
	   }
	*/
	var rawSize int
	err = pbf.ForEachField(bdata, func(f pbf.Field) error {
		switch f.Num() {
		case 1:
			data = bdata
		case 2:
			rawSize = int(f.Uint64())
		case 3:
			data, err = zlibInflateGo(f.Data(), rawSize)
			if err != nil {
				return err
			}
		}
		return nil
	})
	return data, err
}

func zlibInflateGo(data []byte, expectedInflatedSize int) ([]byte, error) {
	rd, err := zlib.NewReader(bytes.NewBuffer(data))
	if err != nil {
		return nil, err
	}
	defer rd.Close()
	out, err := ioutil.ReadAll(rd)
	if err != nil {
		return nil, err
	}
	if len(out) != expectedInflatedSize {
		return nil, errors.New("size mismatch")
	}
	return out, nil
}
