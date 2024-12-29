package fragments

import (
	"encoding/binary"

	"golang.org/x/sys/cpu"
)

type ByteOrder interface {
	byteOrder
	dbusFlag() byte
}

type byteOrder interface {
	binary.ByteOrder
	binary.AppendByteOrder
}

type wrapStd struct {
	byteOrder
}

func (w wrapStd) dbusFlag() byte {
	switch w.byteOrder {
	case binary.BigEndian:
		return 'B'
	case binary.LittleEndian:
		return 'l'
	case binary.NativeEndian:
		if cpu.IsBigEndian {
			return 'B'
		}
		return 'l'
	default:
		panic("unknown ByteOrder, how did you manage to make one of those?")
	}
}

var (
	BigEndian    = wrapStd{binary.BigEndian}
	LittleEndian = wrapStd{binary.LittleEndian}
	NativeEndian = wrapStd{binary.NativeEndian}
)
