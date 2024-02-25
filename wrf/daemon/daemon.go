package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strconv"
)

// runHandler 处理/run GET请求并调用wrf.exe
func runHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Invalid request method", http.StatusMethodNotAllowed)
		return
	}

	// 检查是否是/run路径
	if r.URL.Path != "/run" {
		http.NotFound(w, r)
		return
	}

	// 执行wrf.exe程序
	clusterSize := os.Getenv("CLUSTER_SIZE_ENV")                // 4
	clusterName := os.Getenv("CLUSTER_NAME_ENV")                // mpich
	clusterServiceName := os.Getenv("CLUSTER_SERVICE_NAME_ENV") // wrf
	nHosts, err := strconv.Atoi(clusterSize)
	fmt.Printf("CLUSTER_SIZE_ENV = %s\n", clusterSize)
	fmt.Printf("CLUSTER_NAME_ENV = %s\n", clusterName)
	fmt.Printf("CLUSTER_SERVICE_NAME_ENV = %s\n", clusterServiceName)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to run wrf.exe: %v", err), http.StatusInternalServerError)
		return
	}
	hosts := ""
	for i := 0; i < nHosts; i++ {
		if i > 0 {
			hosts = hosts + ","
		}
		hosts = hosts + fmt.Sprintf("%v-%v.%v", clusterName, i, clusterServiceName)
	}
	fmt.Printf("running on hosts = %s\n", hosts)
	cmd := exec.Command("mpirun", "-hosts", hosts, "-np", clusterSize, "/project/wrf.exe")
	output, err := cmd.Output()
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to run wrf.exe: %v", err), http.StatusInternalServerError)
		return
	}

	// 将wrf.exe的输出作为响应返回
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Write(output)
}

func main() {
	// 设置路由
	http.HandleFunc("/run", runHandler)

	// 启动服务器
	fmt.Println("Starting server on :9090")
	if err := http.ListenAndServe(":9090", nil); err != nil {
		log.Fatal("ListenAndServe: ", err)
	}
}
