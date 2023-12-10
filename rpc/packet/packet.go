package packet

import (
	"encoding/json"
	"errors"
	"github.com/xiwh/hexhub-agent-plugin/util/buf"
	"google.golang.org/protobuf/proto"
)

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
	return DecodePacket(t.mBytes, false)
}

func (t Packet) Bytes() []byte {
	return t.mBytes
}

func (t Packet) Data(v any) error {
	return json.Unmarshal(t.mBytes, v)
}

func (t Packet) ProtoData(v proto.Message) error {
	return proto.Unmarshal(t.mBytes, v)
}

func DecodePacket(bytes []byte, isXor bool) (packet Packet, err error) {
	if isXor {
		for i, mByte := range bytes {
			bytes[i] = mByte ^ byte(i&0xff)
		}
	}
	b := buf.Create(bytes)
	methodLen, _, err := b.ReadUInt16()
	if err != nil {
		return packet, err
	}
	dataLen, _, err := b.ReadUInt32()
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
		dataBytes = EncodePacket(v.(Packet), false)
	case string:
		dataBytes = []byte(v.(string))
	case []byte:
		dataBytes = v.([]byte)
	case interface{}:
		protoMsg, ok := v.(proto.Message)
		var temp []byte
		var err error
		if ok {
			temp, err = proto.Marshal(protoMsg)
		} else {
			temp, err = json.Marshal(v)
		}
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

func Encode(method string, id uint32, v any, isXor bool) ([]byte, error) {
	packet, err := CreatePacket(method, id, v)
	if err != nil {
		return nil, err
	}
	return EncodePacket(packet, isXor), nil
}

func EncodePacket(packet Packet, isXor bool) []byte {
	data := buf.CreateBySize(10 + len(packet.mBytes))
	methodBytes := []byte(packet.method)
	data.WriteUInt16(uint16(len(methodBytes)))
	data.WriteUInt32(uint32(len(packet.mBytes)))
	data.WriteUInt32(packet.mId)
	data.WriteBytes(methodBytes)
	data.WriteBytes(packet.mBytes)
	result := data.Bytes()
	if isXor {
		for i, mByte := range result {
			result[i] = mByte ^ byte(i&0xff)
		}
	}
	return result
}
