package rpc

import (
	"context"
	"net/http"
	"nhooyr.io/websocket"
)

func Accept(w http.ResponseWriter, req *http.Request, ctx context.Context) (Conn, error) {
	wsConn, err := websocket.Accept(w, req, &websocket.AcceptOptions{
		OriginPatterns: []string{"*"},
	})
	wsConn.SetReadLimit(1024 * 128)
	if err != nil {
		return nil, err
	}
	return NewConn(wsConn, ctx), err
}
