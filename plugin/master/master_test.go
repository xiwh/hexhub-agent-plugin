package master

import (
	"github.com/xiwh/gaydev-agent-plugin/plugin"
	"testing"
	"time"
)

func Test(t *testing.T) {
	plugin.SetDebug()
	go func() {
		for true {
			time.Sleep(time.Second)
			Post("test", "ping", nil, nil)
		}
	}()
	Start()
}
