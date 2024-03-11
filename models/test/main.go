package main

import (
	"github.com/clouddea/devs-go/simulation"
	"log"
	"net/rpc"
)

func main() {
	client, err := rpc.DialHTTP("tcp", "localhost:8082")
	if err != nil {
		log.Fatalln("构建rpc客户端失败", err)
	}
	var input = simulation.RootTimeArg{}
	var reply = simulation.RootTimeArg{}
	err = client.Call("Root.RPCSimulate", &input, &reply)
	if err != nil {
		log.Fatalln("通过RPC启动仿真失败", err)
	}
	log.Print("启动仿真成功")
}
