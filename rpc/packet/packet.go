package packet

import (
	"encoding/json"
	"errors"
	"github.com/xiwh/gaydev-agent-plugin/util/buf"
)

const PacketHeadBytes = 8

type Packet struct {
	method string
	mId    uint32
	mBytes []byte
}

func (t Packet) Len() int {
	return len(t.mBytes)
}

func (t Packet) Method() string {
	return t.method
}

func (t Packet) Id() uint32 {
	return t.mId
}

func (t Packet) String() string {
	return string(t.mBytes)
}

func (t Packet) SubPacket() (Packet, error) {
	return DecodePacket(t.mBytes)
}

func (t Packet) Bytes() []byte {
	return t.mBytes
}

func (t Packet) Data(v any) error {
	return json.Unmarshal(t.mBytes, v)
}

func DecodePacket(bytes []byte) (packet Packet, err error) {
	b := buf.Create(bytes)
	methodLen, _, err := b.ReadUInt16()
	if err != nil {
		return packet, err
	}
	dataLen, _, err := b.ReadUInt16()
	if err != nil {
		return packet, err
	}
	id, _, err := b.ReadUInt32()
	if err != nil {
		return packet, err
	}
	method, _, err := b.ReadString(int(methodLen))
	if err != nil {
		return packet, err
	}
	dataBytes, _, err := b.ReadBytes(int(dataLen))
	if err != nil {
		return packet, err
	}
	packet.mBytes = dataBytes
	packet.mId = id
	packet.method = method
	return packet, err
}

func CreatePacket(method string, id uint32, v any) (Packet, error) {
	var dataBytes []byte
	switch v.(type) {
	case Packet:
		dataBytes = EncodePacket(v.(Packet))
	case string:
		dataBytes = []byte(v.(string))
	case []byte:
		dataBytes = v.([]byte)
	case interface{}:
		temp, err := json.Marshal(&v)
		if err != nil {
			return Packet{}, err
		}
		dataBytes = temp
	default:
		return Packet{}, errors.New("value type error")
	}
	return Packet{
		mId:    id,
		method: method,
		mBytes: dataBytes,
	}, nil
}

func Encode(method string, id uint32, v any) ([]byte, error) {
	packet, err := CreatePacket(method, id, v)
	if err != nil {
		return nil, err
	}
	return EncodePacket(packet), nil
}

func EncodePacket(packet Packet) []byte {
	data := buf.CreateBySize(10 + len(packet.mBytes))
	methodBytes := []byte(packet.method)
	data.WriteUInt16(uint16(len(methodBytes)))
	data.WriteUInt16(uint16(len(packet.mBytes)))
	data.WriteUInt32(packet.mId)
	data.WriteBytes(methodBytes)
	data.WriteBytes(packet.mBytes)
	return data.Bytes()
}
