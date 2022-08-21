package rpc

import (
	"context"
	"errors"
	cmap "github.com/orcaman/concurrent-map/v2"
	"github.com/xiwh/gaydev-agent-plugin/rpc/packet"
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
		closeFunc:    nil,
		ctx:          ctx,
		id:           0xffffffff,
	}
	go func() {
		<-ctx.Done()
		if v.err != nil {
			v.triggerClose(v.err)
		} else {
			v.triggerClose(ctx.Err())
		}
	}()

	return v
}

type Conn interface {
	StartHandler(timeout int) error
	Read() (packet.Packet, error)
	Send(method string, v any) error
	SendWaitReply(method string, v any, f func(timeout bool, packet packet.Packet)) error
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
	wsConn       *websocket.Conn
	isClosed     bool
	session      cmap.ConcurrentMap[any]
	handleMap    cmap.ConcurrentMap[func(conn Conn, packet packet.Packet)]
	replyFuncMap cmap.ConcurrentMap[reply]
	closeFunc    func(conn Conn, reason error)
	ctx          context.Context
	id           uint32
	timeout      int64
	err          error
}

func (t *conn) StartHandler(timeout int) error {
	t.timeout = int64(timeout)
	go func() {
		if !t.isClosed {
			time.Sleep(time.Second)
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
	return nil
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

func (t *conn) Send(method string, v any) error {
	miuns := int32(-1)
	uMinuns := uint32(miuns)
	atomic.AddUint32(&t.id, uMinuns)
	return t._send(method, t.id, v)
}

func (t *conn) SendWaitReply(method string, v any, f func(timeout bool, packet packet.Packet)) error {
	miuns := int32(-1)
	uMinuns := uint32(miuns)
	atomic.AddUint32(&t.id, uMinuns)
	err := t._send(method, t.id, v)
	if err != nil {
		t.replyFuncMap.Set(strconv.FormatInt(int64(t.id), 32), reply{
			f:    f,
			time: time.Now().Unix() + t.timeout,
		})
	}
	return err
}

func (t *conn) Reply(method string, v any, packet packet.Packet) error {
	return t._send(method, packet.Id(), v)
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

func (t *conn) _send(method string, id uint32, v any) error {
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
		if t.closeFunc != nil {
			t.closeFunc(t, err)
		}
	}
}
