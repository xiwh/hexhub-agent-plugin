package rpc

import (
	"context"
	"errors"
	"github.com/xiwh/hexhub-agent-plugin/rpc/packet"
	"strconv"
	"sync/atomic"
	"time"
)

const ChannelMethodOpen = "ChannelOpen"
const ChannelMethodSend = "ChannelSend"
const ChannelMethodClose = "ChannelClose"

const CloseNormal = 0
const CloseFailure = 1
const CloseInterrupt = 2

var ChannelClosedError = errors.New("channel is closed")
var TimeoutError = errors.New("timeout")

type Channel struct {
	method          string
	mId             uint32
	ch              chan any
	conn            Conn
	channelIdSerial uint32
	isOpen          bool
	isClosed        bool
	ctx             context.Context
	ctxCancel       func()
}

type CloseInfo struct {
	Code   int    `json:"code"`
	Reason string `json:"reason"`
}

func newChannel(rpcConn Conn, id uint32, method string, ctx context.Context) (*Channel, error) {
	ctx, ctxCancel := context.WithCancel(ctx)
	return &Channel{
		method:          method,
		mId:             id,
		ch:              make(chan any, 4),
		conn:            rpcConn,
		isOpen:          false,
		channelIdSerial: 0,
		ctx:             ctx,
		ctxCancel:       ctxCancel,
	}, nil
}

func (t *Channel) idString() string {
	return strconv.FormatInt(int64(t.mId), 32)
}

func (t *Channel) Id() uint32 {
	return t.mId
}

func (t *Channel) GetContext() context.Context {
	return t.ctx
}

func (t *Channel) IsClosed() bool {
	return t.isClosed
}

func (t *Channel) onOpen() {
	t.isOpen = true
}

func (t *Channel) Close(code int, reason string) error {
	if t.IsClosed() {
		return nil
	}
	t.isOpen = false
	t.isClosed = true
	defer close(t.ch)
	t.ctxCancel()
	return t.conn.SendSpecifyId(ChannelMethodClose, t.mId, CloseInfo{
		code,
		reason,
	})
}

func (t *Channel) ReadTimeout(timeout time.Duration) (packet.Packet, error) {
	if t.IsClosed() {
		return packet.Packet{}, ChannelClosedError
	}
	select {
	case v, ok := <-t.ch:
		if !ok {
			return packet.Packet{}, ChannelClosedError
		}
		switch v.(type) {
		case error:
			return packet.Packet{}, v.(error)
		}
		return v.(packet.Packet), nil
	case <-time.After(timeout):
		return packet.Packet{}, TimeoutError
	}
}

func (t *Channel) Read() (packet.Packet, error) {
	if t.IsClosed() {
		return packet.Packet{}, ChannelClosedError
	}
	v, ok := <-t.ch
	if !ok {
		return packet.Packet{}, ChannelClosedError
	}
	switch v.(type) {
	case error:
		return packet.Packet{}, v.(error)
	}
	return v.(packet.Packet), nil
}

func (t *Channel) Receive(data any) error {
	if t.isClosed {
		return ChannelClosedError
	}
	t.ch <- data
	return nil
}

func (t *Channel) Send(v any) error {
	if t.IsClosed() {
		return ChannelClosedError
	}
	atomic.AddUint32(&t.channelIdSerial, 1)
	return t.conn.SendSpecifyId(ChannelMethodSend, t.mId, v)
}
