package main

import (
	"github.com/xiwh/hexhub-agent-plugin/plugin/master"
	"github.com/xiwh/hexhub-agent-plugin/plugin/slave"
)

func main() {
	master.Start()
	slave.Start("aa")
}
