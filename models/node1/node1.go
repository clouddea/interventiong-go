package main

import (
	"github.com/clouddea/devs-go/examples"
	"github.com/clouddea/devs-go/simulation"
	"intervention-go/config"
)

func main() {
	rootProcStub := simulation.NewProcessorStub(config.CONFIG_ROOT_ENDPOINT)
	// 原子模型仿真节点
	genr := examples.NewGenerator("generator1")
	simulation.NewRoot(simulation.NewSimulator(genr, rootProcStub)).Serve(config.CONFIG_GENR_SERVE)
}
