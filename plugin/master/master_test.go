package master

import (
	"testing"
	"time"
)

func Test(t *testing.T) {
	go func() {
		for true {
			time.Sleep(time.Second)
			Post("test", "ping", nil, nil)
		}
	}()
	Start("hexhub", 0, "1.0.0", "http://localhost:8080", "localhost,127.0.0.1", 35580, true)
}
