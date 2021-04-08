// Copyright 2021 Joshua J Baker. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package pbf

import (
	"encoding/binary"
	"errors"
	"io"
	"math"
	"os"
)

type reader struct {
	err error
	set []byte
	ptr []byte
}

func initReader(bytes []byte) reader {
	rd := reader{
		set: bytes,
		ptr: bytes,
	}
	return rd
}

type fieldType uint64

const (
	typeVarint fieldType = 0
	type64Bit            = 1
	typeLength           = 2
	type32Bit            = 5
)

// Field represents a protobuf field
type Field struct {
	num  uint64
	typ  fieldType
	data []byte
}

// Uint64 returns a uint64 field value
func (f *Field) Uint64() uint64 {
	switch f.typ {
	case type32Bit:
		return uint64(binary.LittleEndian.Uint32(f.data))
	case type64Bit:
		return uint64(binary.LittleEndian.Uint64(f.data))
	case typeVarint:
		x, _ := binary.Uvarint(f.data)
		return x
	}
	return 0
}

// Int64 returns an int64 field value
func (f *Field) Int64() int64 {
	switch f.typ {
	case type32Bit:
		return int64(binary.LittleEndian.Uint32(f.data))
	case type64Bit:
		return int64(binary.LittleEndian.Uint64(f.data))
	case typeVarint:
		x, _ := binary.Varint(f.data)
		return x
	}
	return 0
}

// Float64 returns the float64 field value
func (f *Field) Float64() float64 {
	switch f.typ {
	case type32Bit:
		return float64(math.Float32frombits(binary.LittleEndian.Uint32(f.data)))
	case type64Bit:
		return float64(math.Float64frombits(binary.LittleEndian.Uint64(f.data)))
	}
	return float64(f.Uint64())
}

// Num returns the field number
func (f *Field) Num() uint64 {
	return f.num
}

// Data returns the field data
func (f *Field) Data() []byte {
	if f.typ == typeLength {
		return f.data
	}
	return nil
}

func (rd *reader) readField() (Field, error) {
	if rd.err != nil {
		return Field{}, rd.err
	}
	if len(rd.ptr) == 0 {
		rd.err = io.EOF
		return Field{}, rd.err
	}
	x, n := binary.Uvarint(rd.ptr)
	if n <= 0 {
		rd.err = os.ErrInvalid
		return Field{}, rd.err
	}
	rd.ptr = rd.ptr[n:]
	var field Field
	field.num = x >> 3
	field.typ = fieldType(x & 7)
	switch field.typ {
	case typeVarint:
		fallthrough
	case typeLength:
		x, n = binary.Uvarint(rd.ptr)
		if n <= 0 {
			rd.err = os.ErrInvalid
			return Field{}, rd.err
		}
	case type64Bit:
		n = 8
	case type32Bit:
		n = 4
	default:
		rd.err = errors.New("bad wire type")
		return Field{}, rd.err
	}
	if len(rd.ptr) < n {
		rd.err = io.ErrUnexpectedEOF
		return Field{}, rd.err
	}
	field.data = rd.ptr[:n]
	rd.ptr = rd.ptr[n:]
	if field.typ == typeLength {
		if x > uint64(len(rd.ptr)) {
			rd.err = io.ErrUnexpectedEOF
			return Field{}, rd.err
		}
		field.data = rd.ptr[:x]
		rd.ptr = rd.ptr[x:]
	}
	return field, nil
}

// ForEachField iterates through each field in protobuf bytes
func ForEachField(bytes []byte, iter func(f Field) error) error {
	rd := initReader(bytes)
	for {
		f, err := rd.readField()
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}
		if err := iter(f); err != nil {
			return err
		}
	}
	return nil
}

// ForEachPackedUint64 iterates through each packed uint64
func (f *Field) ForEachPackedUint64(iter func(x uint64) error) error {
	for i := 0; i < len(f.data); {
		x, n := binary.Uvarint(f.data[i:])
		if n <= 0 {
			if n == 0 {
				return errors.New("buffer too small")
			}
			return errors.New("value larget than 64 bits")
		}
		i += n
		if err := iter(x); err != nil {
			return err
		}
	}
	return nil
}

// ForEachPackedInt64 iterates through each packed uint64
func (f *Field) ForEachPackedInt64(iter func(x int64) error) error {
	for i := 0; i < len(f.data); {
		x, n := binary.Varint(f.data[i:])
		if n <= 0 {
			if n == 0 {
				return errors.New("buffer too small")
			}
			return errors.New("value larget than 64 bits")
		}
		i += n
		if err := iter(x); err != nil {
			return err
		}
	}
	return nil
}
