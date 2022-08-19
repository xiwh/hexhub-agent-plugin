package main

import (
	"github.com/xiwh/gaydev-agent-plugin/plugin/master"
	"github.com/xiwh/gaydev-agent-plugin/plugin/slave"
)

func main() {
	master.Start()
	slave.Start("aa")
}
