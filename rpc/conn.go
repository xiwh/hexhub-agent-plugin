package rpc

import (
	"context"
	"errors"
	cmap "github.com/orcaman/concurrent-map/v2"
	"github.com/wonderivan/logger"
	"github.com/xiwh/gaydev-agent-plugin/rpc/packet"
	"math"
	"nhooyr.io/websocket"
	"strconv"
	"sync/atomic"
	"time"
)

func NewConn(wsConn *websocket.Conn, ctx context.Context) Conn {
	v := &conn{
		wsConn:       wsConn,
		isClosed:     false,
		session:      cmap.New[any](),
		handleMap:    cmap.New[func(conn Conn, packet packet.Packet)](),
		replyFuncMap: cmap.New[reply](),
		channelMap:   cmap.New[*Channel](),
		closeFunc:    nil,
		ctx:          ctx,
		id:           0xffffffff,
	}
	return v
}

type Conn interface {
	StartHandler() error
	OpenChannel(method string, v any) (*Channel, error)
	ListenChannel(listen func(conn Conn, p packet.Packet) (*Channel, error))
	Read() (packet.Packet, error)
	Send(method string, v any) (uint32, error)
	SendSpecifyId(method string, id uint32, v any) error
	SendWaitReply(method string, v any, timeout int64, f func(timeout bool, packet packet.Packet)) error
	Reply(method string, v any, packet packet.Packet) error
	Session() cmap.ConcurrentMap[any]
	IsClosed() bool
	Close(err error) error
	HandleFunc(method string, handle func(conn Conn, packet packet.Packet))
	OnClose(f func(conn Conn, err error))
	Ctx() context.Context
}

type reply struct {
	f    func(timeout bool, packet packet.Packet)
	time int64
}

type conn struct {
	wsConn        *websocket.Conn
	isClosed      bool
	session       cmap.ConcurrentMap[any]
	handleMap     cmap.ConcurrentMap[func(conn Conn, packet packet.Packet)]
	channelMap    cmap.ConcurrentMap[*Channel]
	replyFuncMap  cmap.ConcurrentMap[reply]
	closeFunc     func(conn Conn, reason error)
	listenChannel func(conn Conn, p packet.Packet) (*Channel, error)
	ctx           context.Context
	id            uint32
	err           error
}

func (t *conn) StartHandler() error {
	go func() {
		for true {
			if t.isClosed {
				return
			}
			time.Sleep(time.Second)
			err := t.wsConn.Ping(t.ctx)
			if err != nil {
				t.triggerClose(err)
				return
			}
			now := time.Now().Unix()
			t.replyFuncMap.IterCb(func(key string, v reply) {
				if now >= v.time {
					t.replyFuncMap.Remove(key)
					v.f(true, packet.Packet{})
				}
			})
		}
	}()
	for true {
		p, err := t.Read()
		if err != nil {
			_ = t.Close(err)
			return err
		} else {
			if p.Method() == ChannelMethodOpen {
				channelData, ok := t.channelMap.Get(strconv.FormatInt(int64(t.id), 32))
				if ok {
					channelData.OnOpen()
				} else if t.listenChannel != nil {
					listenChannel, err := t.listenChannel(t, p)
					if err != nil {
						logger.Error(err)
					} else {
						listenChannel.OnOpen()
					}
				}
			} else if p.Method() == ChannelMethodClose {
				channelData, ok := t.channelMap.Get(strconv.FormatInt(int64(t.id), 32))
				if ok {
					err := channelData.Close(p.String())
					if err != nil {
						logger.Error(err)
					}
				}
			} else if p.Method() == ChannelMethodSend {
				channelData, ok := t.channelMap.Get(strconv.FormatInt(int64(t.id), 32))
				if ok {
					err := channelData.Receive(p)
					if err != nil {
						logger.Error(err)
					}
				}
			} else {
				reply, ok := t.replyFuncMap.Get(strconv.FormatInt(int64(p.Id()), 32))
				if ok {
					reply.f(false, p)
				} else {
					methodFunc, ok := t.handleMap.Get(p.Method())
					if ok {
						methodFunc(t, p)
					}
				}
			}
		}
	}
	return nil
}

func (t *conn) OpenChannel(method string, v any) (*Channel, error) {
	openChannel, err := OpenChannel(t, method, v, func(channel *Channel) {
		t.channelMap.Remove(channel.IdString())
	})
	if err != nil {
		return nil, err
	}
	t.channelMap.Set(openChannel.IdString(), openChannel)
	return openChannel, nil
}
func (t *conn) ListenChannel(listen func(conn Conn, p packet.Packet) (*Channel, error)) {
	t.listenChannel = listen
}

func (t *conn) Read() (packet.Packet, error) {
	var p packet.Packet
	if t.isClosed {
		return p, errors.New("conn is closed")
	}
	_, b, err := t.wsConn.Read(t.ctx)
	if err != nil {
		return p, err
	}
	return packet.DecodePacket(b)
}

func (t *conn) Send(method string, v any) (uint32, error) {
	minus := int32(-1)
	uMinus := uint32(minus)
	atomic.AddUint32(&t.id, uMinus)
	return t.id, t.SendSpecifyId(method, t.id, v)
}

func (t *conn) SendWaitReply(method string, v any, timeout int64, f func(timeout bool, packet packet.Packet)) error {
	minus := int32(-1)
	uMinus := uint32(minus)
	atomic.AddUint32(&t.id, uMinus)
	err := t.SendSpecifyId(method, t.id, v)
	if err == nil {
		var expire int64 = math.MaxInt64
		if timeout > 0 {
			expire = time.Now().Unix() + timeout
		}
		t.replyFuncMap.Set(strconv.FormatInt(int64(t.id), 32), reply{
			f:    f,
			time: expire,
		})
	}
	return err
}

func (t *conn) Reply(method string, v any, packet packet.Packet) error {
	return t.SendSpecifyId(method, packet.Id(), v)
}

func (t *conn) Session() cmap.ConcurrentMap[any] {
	return t.session
}

func (t *conn) IsClosed() bool {
	return t.isClosed
}

func (t *conn) Close(err error) error {
	if t.isClosed {
		return errors.New("conn is closed")
	}
	t.triggerClose(err)
	return t.wsConn.Close(1000, err.Error())
}

func (t *conn) HandleFunc(method string, handle func(conn Conn, packet packet.Packet)) {
	t.handleMap.Set(method, handle)
}

func (t *conn) OnClose(f func(conn Conn, err error)) {
	t.closeFunc = f
}

func (t *conn) Ctx() context.Context {
	return t.ctx
}

func (t *conn) SendSpecifyId(method string, id uint32, v any) error {
	if t.isClosed {
		return errors.New("conn is closed")
	}
	bytes, err := packet.Encode(method, id, v)
	if err != nil {
		return err
	}
	return t.wsConn.Write(t.ctx, websocket.MessageBinary, bytes)
}

func (t *conn) triggerClose(err error) {
	if !t.isClosed {
		t.isClosed = true
		defer func() {
			_, cancelFunc := context.WithCancel(t.ctx)
			cancelFunc()
		}()
		if t.closeFunc != nil {
			t.closeFunc(t, err)
		}
	}
}
