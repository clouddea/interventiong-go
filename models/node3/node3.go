package main

import (
	"github.com/clouddea/devs-go/examples"
	"github.com/clouddea/devs-go/simulation"
	"intervention-go/config"
)

func main() {
	rootProcStub := simulation.NewProcessorStub(config.CONFIG_ROOT_ENDPOINT)
	// 原子模型仿真节点
	proc := examples.NewProcessor("processor1")
	simulation.NewRoot(simulation.NewSimulator(proc, rootProcStub)).Serve(config.CONFIG_PROC_SERVE)
}
