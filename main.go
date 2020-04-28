package main

import (
    "context"
    "fmt"
    "log"
    "syscall"
    "time"
    "os"
    "path"
    "io/ioutil"

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

    wd, err := os.Getwd()
    writeHostsErr := ioutil.WriteFile(path.Join(wd, "hosts"),
	[]byte(`127.0.0.1	localhost`), 0644)
    if writeHostsErr != nil {
	log.Fatalln(fmt.Errorf("cannot write hosts file: %s", writeHostsErr).Error())
    }

    writeResolvErr := ioutil.WriteFile(path.Join(wd, "resolv.conf"),
	[]byte(`nameserver 8.8.8.8`), 0644)

    if writeResolvErr != nil {
	log.Fatalln(fmt.Errorf("cannot write resolv.conf file: %s", writeResolvErr).Error())
    }    
    //modify runtime
    c_opts_runtime := containerd.WithDefaultRuntime("io.containerd.kata.v2")
    client, err := containerd.New("/run/containerd/containerd.sock", c_opts_runtime)
    log.Println("It's kata runtime")
    //client, err := containerd.New("/run/containerd/containerd.sock")
    if err != nil {
        return err
    }
    defer client.Close()

    ctx := namespaces.WithNamespace(context.Background(), "ctrtest")
    log.Println("before pulling image")
    image, err := client.Pull(ctx, "docker.io/library/redis:alpine", containerd.WithPullUnpack)
    if err != nil {
		return err
    }

    snapshotter := ""
    mounts := getMounts()
    log.Println("before newcontainer")
    container, err := client.NewContainer(
	ctx,
	"katatest",
	containerd.WithImage(image),
	containerd.WithSnapshotter(snapshotter),
	containerd.WithNewSnapshot("katasnapshot", image),
	containerd.WithNewSpec(oci.WithImageConfig(image),
		oci.WithMounts(mounts)),
    )
    log.Println("after newcontainer")
    if err != nil {
	return err
    }
    defer container.Delete(ctx, containerd.WithSnapshotCleanup)
    task, err := container.NewTask(ctx, cio.NewCreator(cio.WithStdio))
    if err != nil {
	return err
    }
    log.Println("after newtask")
    defer task.Delete(ctx)
    log.Printf("[CreateTask]Task ID: %s, Task PID: %d\t\n", task.ID(), task.Pid())

    /*=========Create cni network=======*/
    log.Println("[begin set network]")
    cni, err := handlers.InitNetwork()
    if err != nil {
	panic(err)
    }
    labels := map[string]string{}
    network, err := handlers.CreateCNINetwork(ctx, cni, task, labels)
    if err != nil {
	return err
    }
    ip, err := handlers.GetIPAddress(network, task)
    if err != nil {
	return err
    }
    log.Printf("[Create]This is IP: %s.\n", ip.String())

    exitStatusC, err := task.Wait(ctx)
    if err != nil {
        fmt.Println(err)
    }
    if err := task.Start(ctx); err != nil {
	return err
    }
    log.Println("after task.start")
    time.Sleep(180 * time.Second)
    if err := task.Kill(ctx, syscall.SIGTERM); err != nil {
	return err
    }
    status := <-exitStatusC
    code, _, err := status.Result()
    if err != nil {
	return err
    }
    log.Printf("redis-server exited with status: %d\n", code)

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
