package rpc

import (
	"context"
	"errors"
	"github.com/gorilla/websocket"
	cmap "github.com/orcaman/concurrent-map/v2"
	"github.com/wonderivan/logger"
	"github.com/xiwh/hexhub-agent-plugin/rpc/packet"
	"math"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

var ConnClosedError = errors.New("conn is closed")

func NewConn(wsConn *websocket.Conn, ctx context.Context) Conn {
	ctx, cancel := context.WithCancel(ctx)
	v := &conn{
		wsConn:            wsConn,
		writeLock:         new(sync.Mutex),
		isClosed:          false,
		session:           cmap.New[any](),
		handleMap:         cmap.New[handleFunc](),
		replyFuncMap:      cmap.New[reply](),
		channelMap:        cmap.New[*Channel](),
		channelAcceptChan: make(chan packet.Packet, 128),
		closeFunc:         nil,
		ctx:               ctx,
		ctxCancel:         cancel,
		id:                0xffffffff,
	}
	return v
}

type Conn interface {
	StartHandler() error
	OpenChannel(method string, v any) (*Channel, error)
	AcceptChannel() (packet.Packet, *Channel, error)
	Read() (packet.Packet, error)
	Send(method string, v any) (uint32, error)
	SendSpecifyId(method string, id uint32, v any) error
	SendWaitReply(method string, v any, timeout int64, f func(timeout bool, packet packet.Packet)) error
	Reply(method string, v any, packet packet.Packet) error
	Session() cmap.ConcurrentMap[any]
	IsClosed() bool
	Close(err error) error
	HandleFunc(method string, handle func(conn Conn, packet packet.Packet))
	HandleFuncAsync(method string, handle func(conn Conn, packet packet.Packet))
	OnClose(f func(conn Conn, err error))
	Ctx() context.Context
}

type reply struct {
	f    func(timeout bool, packet packet.Packet)
	time int64
}

type handleFunc struct {
	isAsync bool
	handle  func(conn Conn, packet packet.Packet)
}

type conn struct {
	wsConn            *websocket.Conn
	writeLock         *sync.Mutex
	isClosed          bool
	session           cmap.ConcurrentMap[any]
	handleMap         cmap.ConcurrentMap[handleFunc]
	channelMap        cmap.ConcurrentMap[*Channel]
	replyFuncMap      cmap.ConcurrentMap[reply]
	closeFunc         func(conn Conn, reason error)
	channelAcceptChan chan packet.Packet
	ctx               context.Context
	ctxCancel         func()
	id                uint32
	err               error
}

func (t *conn) StartHandler() error {
	go func() {
		for true {
			if t.isClosed {
				return
			}
			time.Sleep(time.Second)
			t.writeLock.Lock()
			err := t.wsConn.WriteMessage(websocket.PingMessage, []byte("ping"))
			t.writeLock.Unlock()
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
				channelData, ok := t.channelMap.Get(strconv.FormatInt(int64(p.Id()), 32))
				if ok {
					channelData.onOpen()
				} else {
					t.channelAcceptChan <- p
				}
			} else if p.Method() == ChannelMethodClose {
				channelData, ok := t.channelMap.Get(strconv.FormatInt(int64(p.Id()), 32))
				if ok {
					var closeInfo CloseInfo
					err = p.Data(&closeInfo)
					if err != nil {
						logger.Error(err)
						err := channelData.Close(CloseFailure, err.Error())
						if err != nil {
							logger.Error(err)
						}
					} else {
						err = channelData.Close(closeInfo.Code, closeInfo.Reason)
						if err != nil {
							logger.Error(err)
						}
					}
				}
			} else if p.Method() == ChannelMethodSend {
				channelData, ok := t.channelMap.Get(strconv.FormatInt(int64(p.Id()), 32))
				if ok {
					_ = channelData.Receive(p)
					//if err != nil {
					//	logger.Error(err)
					//}
				}
			} else {
				reply, ok := t.replyFuncMap.Get(strconv.FormatInt(int64(p.Id()), 32))
				if ok {
					reply.f(false, p)
				} else {
					handle, ok := t.handleMap.Get(p.Method())
					if ok {
						if handle.isAsync {
							go handle.handle(t, p)
						} else {
							handle.handle(t, p)
						}
					}
				}
			}
		}
	}
	return nil
}

func (t *conn) OpenChannel(method string, v any) (*Channel, error) {
	openPacket, err := packet.CreatePacket(method, 0, v)
	if err != nil {
		return nil, err
	}
	id, err := t.Send(ChannelMethodOpen, openPacket)
	if err != nil {
		return nil, err
	}
	ctx := context.Background()

	openChannel, err := newChannel(t, id, method, ctx)
	if err != nil {
		return nil, err
	}
	t.channelMap.Set(openChannel.idString(), openChannel)
	go func() {
		<-ctx.Done()
		t.channelMap.Remove(openChannel.idString())
	}()
	return openChannel, nil
}

func (t *conn) AcceptChannel() (packet.Packet, *Channel, error) {
	p, ok := <-t.channelAcceptChan
	if !ok {
		return packet.Packet{}, nil, ConnClosedError
	}
	ctx := context.Background()
	subPacket, err := p.SubPacket()
	if err != nil {
		return subPacket, nil, err
	}

	openChannel, err := newChannel(t, p.Id(), subPacket.Method(), ctx)
	if err == nil {
		t.channelMap.Set(openChannel.idString(), openChannel)
		go func() {
			<-ctx.Done()
			t.channelMap.Remove(openChannel.idString())
		}()
		err := t.SendSpecifyId(p.Method(), p.Id(), p.Bytes())
		if err != nil {
			logger.Error(err)
		}
	} else {
		err := t.SendSpecifyId(ChannelMethodClose, p.Id(), err.Error())
		if err != nil {
			logger.Error(err)
		}
	}
	return p, openChannel, err
}

func (t *conn) Read() (packet.Packet, error) {
	var p packet.Packet
	if t.isClosed {
		return p, ConnClosedError
	}
	msgType, b, err := t.wsConn.ReadMessage()
	if err != nil {
		return p, err
	}
	switch msgType {
	case websocket.BinaryMessage:
		return packet.DecodePacket(b, true)
	case websocket.PingMessage:
		t.writeLock.Lock()
		_ = t.wsConn.WriteMessage(websocket.PongMessage, []byte("pong"))
		t.writeLock.Unlock()
		return t.Read()
	case websocket.PongMessage:
		return t.Read()
	case websocket.CloseMessage:
		closed := errors.New("closed")
		t.triggerClose(closed)
		return packet.Packet{}, closed

	}
	return packet.Packet{}, errors.New("read failure")
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
		return ConnClosedError
	}
	defer t.triggerClose(err)
	defer func() {
		t.channelMap.IterCb(func(k string, v *Channel) {
			err := v.Close(CloseInterrupt, "rpc connection closed")
			if err != nil {
				logger.Error(err)
			}
		})
		t.channelMap.Clear()
	}()
	msg := err.Error()
	msgBytes := []byte(msg)
	//ws关闭原因最大125字节，超过120字节截取并省略
	if len(msgBytes) > 120 {
		msgBytes = msgBytes[0:120]
		//utf-8为非定长编码，按固定长度截取字节最后一个字编码可能被破坏需要删除，并且在最后添加省略号
		msg = strings.ToValidUTF8(string(msgBytes), "") + ".."
	}
	t.writeLock.Lock()
	defer t.writeLock.Unlock()
	return t.wsConn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, msg))
}

func (t *conn) HandleFuncAsync(method string, handle func(conn Conn, packet packet.Packet)) {
	t.handleMap.Set(method, handleFunc{true, handle})
}

func (t *conn) HandleFunc(method string, handle func(conn Conn, packet packet.Packet)) {
	t.handleMap.Set(method, handleFunc{false, handle})
}

func (t *conn) OnClose(f func(conn Conn, err error)) {
	t.closeFunc = f
}

func (t *conn) Ctx() context.Context {
	return t.ctx
}

func (t *conn) SendSpecifyId(method string, id uint32, v any) error {
	if t.isClosed {
		return ConnClosedError
	}
	bytes, err := packet.Encode(method, id, v, true)
	if err != nil {
		return err
	}
	t.writeLock.Lock()
	defer t.writeLock.Unlock()
	return t.wsConn.WriteMessage(websocket.BinaryMessage, bytes)
}

func (t *conn) triggerClose(err error) {
	if !t.isClosed {
		t.isClosed = true
		defer func() {
			t.ctxCancel()
		}()
		if t.closeFunc != nil {
			t.closeFunc(t, err)
		}
	}
}
