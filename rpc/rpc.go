package rpc

import (
	"context"
	"github.com/gorilla/websocket"
	"net/http"
)

func Accept(w http.ResponseWriter, req *http.Request, ctx context.Context, readLimit int64) (Conn, error) {
	var upgrader = websocket.Upgrader{
		ReadBufferSize:    0x1fff,
		WriteBufferSize:   0x1fff,
		EnableCompression: true,
		CheckOrigin:       func(r *http.Request) bool { return true },
	}

	wsConn, err := upgrader.Upgrade(w, req, nil)
	if err != nil {
		return nil, err
	}
	wsConn.SetReadLimit(readLimit)

	return NewConn(wsConn, ctx), err
}
