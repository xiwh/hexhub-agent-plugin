package buf

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math"
)

type BufUtil struct {
	data     []byte
	readIdx  int
	writeIdx int
}

func Create(data []byte) *BufUtil {
	buf := new(BufUtil)
	buf.data = data
	buf.writeIdx = len(data)
	return buf
}

func CreateBySize(size int) *BufUtil {
	buf := new(BufUtil)
	buf.data = make([]byte, 0, size)
	buf.readIdx = 0
	return buf
}

func CreateByHexStr(data string) (*BufUtil, error) {
	b, err := hex.DecodeString(data)
	if err != nil {
		return nil, err
	}
	return Create(b), nil
}

func (t *BufUtil) DumpHex() string {
	return hex.Dump(t.data)
}

func (t *BufUtil) ToHex() string {
	return hex.EncodeToString(t.data)
}

func (t *BufUtil) Bytes() []byte {
	return t.data[0:t.writeIdx]
}

func (t *BufUtil) Clear() {
	t.data = make([]byte, 0)
	t.readIdx = 0
	t.writeIdx = 0
}

func (t *BufUtil) SetReadIdx(idx int) {
	t.readIdx = idx
}

func (t *BufUtil) Len() int {
	return len(t.data)
}

func (t *BufUtil) Index() (int, int) {
	return t.readIdx, t.writeIdx
}

func (t *BufUtil) WriteIndex() int {
	return t.writeIdx
}

func (t *BufUtil) ReadIndex() int {
	return t.readIdx
}

func (t *BufUtil) WriteByte(v byte) int {
	t.data = append(t.data, v)
	t.writeIdx += 1
	return t.writeIdx
}

func (t *BufUtil) WriteBytes(v []byte) int {
	t.data = append(t.data, v...)
	t.writeIdx += len(v)
	return t.writeIdx
}

func (t *BufUtil) WriteInt16(v int16) int {
	var b = make([]byte, 2)
	binary.BigEndian.PutUint16(b, uint16(v))
	return t.WriteBytes(b)
}

func (t *BufUtil) WriteUInt16(v uint16) int {
	var b = make([]byte, 2)
	binary.BigEndian.PutUint16(b, v)
	return t.WriteBytes(b)
}

func (t *BufUtil) WriteInt32(v int32) int {
	var b = make([]byte, 4)
	binary.BigEndian.PutUint32(b, uint32(v))
	return t.WriteBytes(b)
}

func (t *BufUtil) WriteUInt32(v uint32) int {
	var b = make([]byte, 4)
	binary.BigEndian.PutUint32(b, v)
	return t.WriteBytes(b)
}

func (t *BufUtil) WriteInt64(v int64) int {
	var b = make([]byte, 4)
	binary.BigEndian.PutUint64(b, uint64(v))
	return t.WriteBytes(b)
}

func (t *BufUtil) WriteUInt64(v uint64) int {
	var b = make([]byte, 4)
	binary.BigEndian.PutUint64(b, v)
	return t.WriteBytes(b)
}

func (t *BufUtil) WriteInt(v int) int {
	var b = make([]byte, 8)
	binary.BigEndian.PutUint64(b, uint64(v))
	return t.WriteBytes(b)
}

func (t *BufUtil) WriteUInt(v uint) int {
	var b = make([]byte, 4)
	binary.BigEndian.PutUint64(b, uint64(v))
	return t.WriteBytes(b)
}

func (t *BufUtil) WriteFloat32(v float32) int {
	var b = make([]byte, 4)
	binary.BigEndian.PutUint32(b, math.Float32bits(v))
	return t.WriteBytes(b)
}

func (t *BufUtil) WriteFloat64(v float64) int {
	var b = make([]byte, 8)
	binary.BigEndian.PutUint64(b, math.Float64bits(v))
	return t.WriteBytes(b)
}

func (t *BufUtil) ReadByte() (byte, int, error) {
	var idx = t.readIdx
	if idx >= t.writeIdx {
		return 0, -1, fmt.Errorf("array out of bounds,len:%d,idx:%d", idx+1, t.writeIdx)
	}
	t.readIdx += 1
	return t.data[idx], t.readIdx, nil
}

func (t *BufUtil) ReadBytes(len int) ([]byte, int, error) {
	var idx = t.readIdx
	if idx+len > t.writeIdx {
		return nil, -1, fmt.Errorf("array out of bounds,len:%d,idx:%d", idx+len-1, t.writeIdx)
	}
	t.readIdx += len
	return t.data[idx : idx+len], t.readIdx, nil
}

func (t *BufUtil) ReadInt16() (int16, int, error) {
	data, idx, err := t.ReadBytes(2)
	if err != nil {
		return 0, idx, err
	}
	return int16(binary.BigEndian.Uint16(data)), idx, nil
}

func (t *BufUtil) ReadUInt16() (uint16, int, error) {
	data, idx, err := t.ReadBytes(2)
	if err != nil {
		return 0, idx, err
	}
	return binary.BigEndian.Uint16(data), idx, nil
}

func (t *BufUtil) ReadInt32() (int32, int, error) {
	data, idx, err := t.ReadBytes(4)
	if err != nil {
		return 0, idx, err
	}
	return int32(binary.BigEndian.Uint32(data)), idx, nil
}

func (t *BufUtil) ReadUInt32() (uint32, int, error) {
	data, idx, err := t.ReadBytes(4)
	if err != nil {
		return 0, idx, err
	}
	return binary.BigEndian.Uint32(data), idx, nil
}

func (t *BufUtil) ReadInt64() (int64, int, error) {
	data, idx, err := t.ReadBytes(8)
	if err != nil {
		return 0, idx, err
	}
	return int64(binary.BigEndian.Uint64(data)), idx, nil
}

func (t *BufUtil) ReadUInt64() (uint64, int, error) {
	data, idx, err := t.ReadBytes(8)
	if err != nil {
		return 0, idx, err
	}
	return binary.BigEndian.Uint64(data), idx, nil
}

func (t *BufUtil) ReadInt() (int, int, error) {
	data, idx, err := t.ReadBytes(8)
	if err != nil {
		return 0, idx, err
	}
	return int(binary.BigEndian.Uint64(data)), idx, nil
}

func (t *BufUtil) ReadUInt() (uint, int, error) {
	data, idx, err := t.ReadBytes(8)
	if err != nil {
		return 0, idx, err
	}
	return uint(binary.BigEndian.Uint64(data)), idx, nil
}

func (t *BufUtil) ReadFloat32() (float32, int, error) {
	v, idx, err := t.ReadUInt32()
	if err != nil {
		return 0, idx, err
	}
	return math.Float32frombits(v), idx, nil
}

func (t *BufUtil) ReadFloat64() (float64, int, error) {
	v, idx, err := t.ReadUInt64()
	if err != nil {
		return 0, idx, err
	}
	return math.Float64frombits(v), idx, nil
}

func (t *BufUtil) ReadString(byteLen int) (string, int, error) {
	bytes, idx, err := t.ReadBytes(byteLen)
	if err != nil {
		return "", idx, err
	}
	return string(bytes), idx, nil
}
