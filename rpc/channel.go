package rpc

import (
	"errors"
	"github.com/xiwh/gaydev-agent-plugin/rpc/packet"
	"strconv"
	"sync/atomic"
)

const ChannelMethodOpen = "ChannelOpen"
const ChannelMethodSend = "ChannelSend"
const ChannelMethodClose = "ChannelClose"

type Channel struct {
	mId             uint32
	ch              chan any
	conn            Conn
	channelIdSerial uint32
	isOpen          bool
	isClose         bool
	onClose         func(channel *Channel)
}

func openChannel(rpcConn Conn, method string, v any) (*Channel, error) {
	openPacket, err := packet.CreatePacket(method, 0, v)
	if err != nil {
		return nil, err
	}
	id, err := rpcConn.Send(ChannelMethodOpen, openPacket)
	if err != nil {
		return nil, err
	}
	return &Channel{
		mId:             id,
		ch:              make(chan any, 4),
		conn:            rpcConn,
		isOpen:          false,
		isClose:         false,
		channelIdSerial: 0,
	}, nil
}

func (t Channel) ListenClose(f func(channel *Channel)) {
	t.onClose = f
}

func (t *Channel) IdString() string {
	return strconv.FormatInt(int64(t.mId), 32)
}

func (t *Channel) Id() uint32 {
	return t.mId
}

func (t *Channel) IsClosed() bool {
	return t.isClose
}

func (t *Channel) OnOpen() {
	t.isClose = true
}

func (t *Channel) Close(reason string) error {
	t.isOpen = false
	t.isClose = true
	close(t.ch)
	t.onClose(t)
	return t.conn.SendSpecifyId(ChannelMethodClose, t.mId, reason)
}

func (t *Channel) Read() (packet.Packet, error) {
	if t.IsClosed() {
		return packet.Packet{}, errors.New("channel is closed")
	}
	v := <-t.ch
	switch v.(type) {
	case error:
		return packet.Packet{}, v.(error)
	}
	return v.(packet.Packet), nil
}

func (t *Channel) Receive(data any) error {
	if t.IsClosed() {
		return errors.New("channel is closed")
	}
	t.ch <- data
	return nil
}

func (t *Channel) Send(v any) error {
	if t.IsClosed() {
		return errors.New("channel is closed")
	}
	atomic.AddUint32(&t.channelIdSerial, 1)
	return t.conn.SendSpecifyId(ChannelMethodSend, t.mId, v)
}
