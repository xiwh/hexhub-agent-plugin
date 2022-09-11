package rpc

import (
	"context"
	"errors"
	"github.com/xiwh/gaydev-agent-plugin/rpc/packet"
	"strconv"
	"sync/atomic"
)

const ChannelMethodOpen = "ChannelOpen"
const ChannelMethodSend = "ChannelSend"
const ChannelMethodClose = "ChannelClose"

const CloseNormal = 0
const CloseFailure = 1
const CloseInterrupt = 2

type Channel struct {
	method          string
	mId             uint32
	ch              chan any
	conn            Conn
	channelIdSerial uint32
	isOpen          bool
	ctx             context.Context
}

type CloseInfo struct {
	Code   int    `json:"code"`
	Reason string `json:"reason"`
}

func newChannel(rpcConn Conn, id uint32, method string, ctx context.Context) (*Channel, error) {
	return &Channel{
		method:          method,
		mId:             id,
		ch:              make(chan any, 4),
		conn:            rpcConn,
		isOpen:          false,
		channelIdSerial: 0,
		ctx:             ctx,
	}, nil
}

func (t *Channel) idString() string {
	return strconv.FormatInt(int64(t.mId), 32)
}

func (t *Channel) Id() uint32 {
	return t.mId
}

func (t *Channel) IsClosed() bool {
	return t.ctx.Err() != nil
}

func (t *Channel) onOpen() {
	t.isOpen = true
}

func (t *Channel) Close(code int, reason string) error {
	t.isOpen = false
	close(t.ch)
	_, cancel := context.WithCancel(t.ctx)
	defer cancel()
	return t.conn.SendSpecifyId(ChannelMethodClose, t.mId, CloseInfo{
		code,
		reason,
	})
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
