package main

import (
    "context"
	"fmt"
	"log"
	"syscall"
	"time"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/cio"
	"github.com/containerd/containerd/oci"
	"github.com/containerd/containerd/namespaces"
)

func main() {
        if err := testExample(); err != nil {
                log.Fatal(err)
        }
}

func testExample() error {

    //modify runtime
    c_opts_runtime := containerd.WithDefaultRuntime("io.containerd.kata.v2")
    client, err := containerd.New("/run/containerd/containerd.sock", c_opts_runtime)
    fmt.Println("this is kata runtime")
    //client, err := containerd.New("/run/containerd/containerd.sock")
    if err != nil {
        return err
    }
    defer client.Close()
   
    ctx := namespaces.WithNamespace(context.Background(), "katatest")   
    fmt.Println("this is before pull image")
    image, err := client.Pull(ctx, "docker.io/library/redis:alpine", containerd.WithPullUnpack)
    if err != nil {
		return err
	}
	// mounts := getMounts()
	// mounts TODO()
	container, err := client.NewContainer(
		ctx,
		"redistest",
		containerd.WithImage(image),
		containerd.WithNewSnapshot("redis-snapshot", image),
		containerd.WithNewSpec(oci.WithImageConfig(image)),
	)
	if err != nil {
		return err
	}
	defer container.Delete(ctx, containerd.WithSnapshotCleanup)
	
	task, err := container.NewTask(ctx, cio.NewCreator(cio.WithStdio))
	if err != nil {
		return err
	}
	defer task.Delete(ctx)
	log.Printf("[CreateTask]Task ID: %s\tTask PID: %d\t\n", task.ID(), task.Pid())
	//Create cni network TODO()
	
	exitStatusC, err := task.Wait(ctx)
	if err != nil {
		fmt.Println(err)
	}
	if err := task.Start(ctx); err != nil {
		return err
	}
	time.Sleep(3 * time.Second)
	if err := task.Kill(ctx, syscall.SIGTERM); err != nil {
		return err
	}
	status := <-exitStatusC
	code, _, err := status.Result()
	if err != nil {
		return err
	}
	fmt.Printf("redis-server exited with status: %d\n", code)

	return nil
}
