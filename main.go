package main

import (
    "context"
	"fmt"
	"log"
	"syscall"
	"time"
	"os"
	"path"

	"katatest/handlers"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/cio"
	"github.com/containerd/containerd/oci"
	"github.com/containerd/containerd/namespaces"

	"github.com/opencontainers/runtime-spec/specs-go"
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

    ctx := namespaces.WithNamespace(context.Background(), "ctrtest")
    fmt.Println("this is before pull image")
    //image, err := client.Pull(ctx, "docker.io/alexellis2/ping:0.1", containerd.WithPullUnpack)
    image, err := client.Pull(ctx, "docker.io/library/redis:alpine", containerd.WithPullUnpack)
    //docker.io/library/redis:alpine
    if err != nil {
		return err
	}

	snapshotter := ""
	mounts := getMounts()
	fmt.Println("this is before newcontainer")
	container, err := client.NewContainer(
		ctx,
		"katatest2",
		containerd.WithImage(image),
		containerd.WithSnapshotter(snapshotter),
		containerd.WithNewSnapshot("katasnapshot", image),
		containerd.WithNewSpec(oci.WithImageConfig(image),
			/*oci.WithCapabilities([]string{"CAP_NET_RAW"}),*/
			oci.WithMounts(mounts)),
	)
	fmt.Println("this is after newcontainer")
	if err != nil {
		return err
	}
	defer container.Delete(ctx, containerd.WithSnapshotCleanup)
	fmt.Println("this is before newtask")
	task, err := container.NewTask(ctx, cio.NewCreator(cio.WithStdio))
	if err != nil {
		return err
	}
	fmt.Println("this is after newtask")
	defer task.Delete(ctx)
	log.Printf("[CreateTask]Task ID: %s, Task PID: %d\t\n", task.ID(), task.Pid())

	//Create cni network TODO()
	fmt.Println("[[[[[[[[[[[begin set network]]]]]]]]]")
	cni, err := handlers.InitNetwork()
	if err != nil {
		panic(err)
	}
	labels := map[string]string{}
	network, err := handlers.CreateCNINetwork(ctx, cni, task, labels)
    fmt.Println("[CreateTask] this is network interface: ", network.Interfaces)
	if err != nil {
		return err
	}
	ip, err := handlers.GetIPAddress(network, task)
	if err != nil {
		return err
	}
	log.Printf("[[[[[set network finish]]]]]]]]This is IP: %s.\n", ip.String())

	exitStatusC, err := task.Wait(ctx)
	if err != nil {
		fmt.Println(err)
	}
	fmt.Println("this is after wait task")
	if err := task.Start(ctx); err != nil {
		return err
	}
	fmt.Println("this is after start task")
	time.Sleep(20 * time.Second)
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

func getMounts() []specs.Mount {
	wd, _ := os.Getwd()
	mounts := []specs.Mount{}
	mounts = append(mounts, specs.Mount{
		Destination: "/etc/resolv.conf",
		Type:        "bind",
		Source:      path.Join(wd, "resolv.conf"),
		Options:     []string{"rbind", "ro"},
	})

	mounts = append(mounts, specs.Mount{
		Destination: "/etc/hosts",
		Type:        "bind",
		Source:      path.Join(wd, "hosts"),
		Options:     []string{"rbind", "ro"},
	})
	return mounts
}
