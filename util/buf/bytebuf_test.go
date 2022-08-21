package buf

import "testing"

func TestByteBuf(t *testing.T) {
	buf := Create([]byte{0, 1, 5, 0, 0, 5})
	println(buf.ReadUInt16())
	println(buf.ReadUInt32())
}
