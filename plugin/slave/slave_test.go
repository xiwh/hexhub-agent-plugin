package slave

import (
	"testing"
	"time"
)

func Test(t *testing.T) {
	//plugin.SetDebug()

	go func() {
		for true {
			time.Sleep(time.Second)
			Post("test", "ping", nil, nil)
			Post("", "ping", nil, nil)

		}
	}()
	Start()
}
