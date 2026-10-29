package agentarchive

import (
	"bytes"
	"encoding/binary"
	"io"
)

// Bound the real central directory before archive/zip allocates File objects.
func preflight(src io.ReaderAt, size int64) error {
	if size < 22 {
		return invalid("truncated ZIP")
	}
	tailSize := int64(65557)
	if size < tailSize {
		tailSize = size
	}
	tail := make([]byte, tailSize)
	if _, e := src.ReadAt(tail, size-tailSize); e != nil {
		return invalid("truncated ZIP")
	}
	end := -1
	for i := len(tail) - 22; i >= 0; i-- {
		if bytes.Equal(tail[i:i+4], []byte{'P', 'K', 5, 6}) && i+22+int(binary.LittleEndian.Uint16(tail[i+20:])) <= len(tail) {
			// Match archive/zip's latest fitting candidate; never validate an earlier directory instead.
			if i+22+int(binary.LittleEndian.Uint16(tail[i+20:])) != len(tail) {
				return invalid("ambiguous or trailing ZIP end record")
			}
			end = i
			break
		}
	}
	if end < 0 {
		return invalid("missing ZIP end record")
	}
	h := tail[end:]
	u16 := binary.LittleEndian.Uint16
	u32 := binary.LittleEndian.Uint32
	u64 := binary.LittleEndian.Uint64
	if u16(h[4:]) != 0 || u16(h[6:]) != 0 || u16(h[8:]) != u16(h[10:]) {
		return invalid("split ZIP unsupported")
	}
	count := uint64(u16(h[10:]))
	length := uint64(u32(h[12:]))
	offset := uint64(u32(h[16:]))
	endOffset := size - tailSize + int64(end)
	if count == 65535 || length == 0xffffffff || offset == 0xffffffff {
		if endOffset < 20 {
			return invalid("missing ZIP64 locator")
		}
		locator := make([]byte, 20)
		if _, e := src.ReadAt(locator, endOffset-20); e != nil {
			return invalid("bad ZIP64 locator")
		}
		if u32(locator) != 0x07064b50 || u32(locator[4:]) != 0 || u32(locator[16:]) != 1 {
			return invalid("split ZIP64 unsupported")
		}
		recordOffset := u64(locator[8:])
		if recordOffset > uint64(endOffset-20) || uint64(endOffset-20)-recordOffset < 56 {
			return invalid("bad ZIP64 offset")
		}
		record := make([]byte, 56)
		if _, e := src.ReadAt(record, int64(recordOffset)); e != nil {
			return invalid("bad ZIP64 record")
		}
		recordSize := u64(record[4:])
		if u32(record) != 0x06064b50 || recordSize < 44 || recordSize > uint64(endOffset-20)-recordOffset-12 || u32(record[16:]) != 0 || u32(record[20:]) != 0 || u64(record[24:]) != u64(record[32:]) {
			return invalid("bad ZIP64 record")
		}
		count = u64(record[32:])
		length = u64(record[40:])
		offset = u64(record[48:])
		endOffset = int64(recordOffset)
	}
	if count > MaxEntries || length > 2<<20 {
		return ErrTooLarge
	}
	if offset > uint64(endOffset) || length > uint64(endOffset)-offset || offset+length != uint64(endOffset) {
		return invalid("invalid central directory bounds")
	}
	pos := int64(offset)
	stop := pos + int64(length)
	var actual uint64
	header := make([]byte, 46)
	for pos < stop {
		if stop-pos < 46 {
			return invalid("truncated central directory")
		}
		if _, e := src.ReadAt(header, pos); e != nil {
			return invalid("truncated central directory")
		}
		if u32(header) != 0x02014b50 {
			return invalid("bad central directory record")
		}
		actual++
		if actual > MaxEntries {
			return ErrTooLarge
		}
		span := int64(46) + int64(u16(header[28:])) + int64(u16(header[30:])) + int64(u16(header[32:]))
		if span > stop-pos {
			return invalid("invalid central record length")
		}
		pos += span
	}
	if actual != count {
		return invalid("inconsistent ZIP entry count")
	}
	return nil
}
