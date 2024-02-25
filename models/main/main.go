package main

import (
	"fmt"
	"github.com/clouddea/devs-go/modeling"
	"github.com/clouddea/devs-go/simulation"
	"intervention-go/config"
	"time"
)

func main() {
	// 耦合模型节点
	genrStub := modeling.NewEntityRemote("generator1", config.CONFIG_GENR_ENDPOINT)
	transStub := modeling.NewEntityRemote("transmitter1", config.CONFIG_TRANS_ENDPOINT)
	procStub := modeling.NewEntityRemote("processor1", config.CONFIG_PROC_ENDPOINT)
	coupled := &modeling.AbstractCoupled{}
	coupled.AddComponent(genrStub)
	coupled.AddComponent(transStub)
	coupled.AddComponent(procStub)
	coupled.AddCoupling(genrStub, "out", transStub, "in")
	coupled.AddCoupling(transStub, "out", procStub, "in1")
	coordinator := simulation.NewCoordinator(coupled, nil)
	root := simulation.NewRoot(coordinator)
	go root.Serve(config.CONFIG_ROOT_SERVE)

	root.Simulate(1*time.Second, func(t uint64) {
		fmt.Printf("time advance: %v \n", t)
	})
}
