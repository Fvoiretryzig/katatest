package handlers

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"os"
	"path"
	"path/filepath"

	"github.com/containerd/containerd"
	gocni "github.com/containerd/go-cni"
	"github.com/pkg/errors"
)

const (
	CNIBinDir = "/opt/cni/bin"
	CNIConfDir = "/etc/cni/net.d"
	NetNSPathFmt = "/proc/%d/ns/net"
	defaultCNIConfFilename = "10-mynet.conflist"
	defaultNetworkName = "openfaas-cni-bridge"
	defaultBridgeName = "openfaas0"
	defaultSubnet = "10.62.0.0/16"
)

var defaultCNIConf = fmt.Sprintf(`
{
    "cniVersion": "0.4.0",
    "name": "%s",
    "plugins": [
      {
        "type": "bridge",
        "bridge": "%s",
        "isGateway": true,
        "ipMasq": true,
        "ipam": {
            "type": "host-local",
            "subnet": "%s",
            "routes": [
                { "dst": "0.0.0.0/0" }
            ]
        }
      },
      {
        "type": "firewall"
      }
    ]
}
`, defaultNetworkName, defaultBridgeName, defaultSubnet)

func InitNetwork() (gocni.CNI, error) {
	log.Printf("Writing network config...\n")
	if !dirExists(CNIConfDir) {
		if err := os.MkdirAll(CNIConfDir, 0755); err != nil {
			return nil, fmt.Errorf("cannot create directory: %s", CNIConfDir)
		}
	}

	netConfig := path.Join(CNIConfDir, defaultCNIConfFilename)
	if err := ioutil.WriteFile(netConfig, []byte(defaultCNIConf), 644); err != nil {
		return nil, fmt.Errorf("cannot write network config: %s", defaultCNIConfFilename)

	}
	// Initialize CNI library
	cni, err := gocni.New(gocni.WithPluginConfDir(CNIConfDir),
		gocni.WithPluginDir([]string{CNIBinDir}))

	if err != nil {
		return nil, fmt.Errorf("error initializing cni: %s", err)
	}

	// Load the cni configuration
	if err := cni.Load(gocni.WithLoNetwork, gocni.WithConfListFile(filepath.Join(CNIConfDir, defaultCNIConfFilename))); err != nil {
		return nil, fmt.Errorf("failed to load cni configuration: %v", err)
	}
	return cni, nil
}

func CreateCNINetwork(ctx context.Context, cni gocni.CNI, task containerd.Task, labels map[string]string) (*gocni.CNIResult, error) {
    fmt.Println("this is CreateCNINetwork!!!")
    fmt.Println("this is map labels: ", labels)
    fmt.Println("this is context\n", ctx)
	id := NetID(task)
	netns := NetNamespace(task)
	fmt.Println("this is netid: ", id)
	fmt.Println("this is netns: ", netns)
	result, err := cni.Setup(ctx, id, netns, gocni.WithLabels(labels))
	if err != nil {
		return nil, errors.Wrapf(err, "Failed to setup network for task %q: %v", id, err)
	}
    fmt.Println("result: dns: " ,result.DNS)
    fmt.Println("result: routetable: ", result.Routes)
    for key, value := range result.Interfaces {
        fmt.Print("key: ", key)
        fmt.Print(" intf mac: ", value.Mac)
        fmt.Print(" intf sandbox: ", value.Sandbox, "                ")
        for i := range result.Interfaces[key].IPConfigs {
            fmt.Print(" IPConfigs: IP: ", result.Interfaces[key].IPConfigs[i].IP, " gw: ", result.Interfaces[key].IPConfigs[i].Gateway, " ")
        }
        fmt.Println("\n")
    }
	return result, nil
}

// DeleteCNINetwork deletes a CNI network based on task ID and Pid
func DeleteCNINetwork(ctx context.Context, cni gocni.CNI, client *containerd.Client, name string) error {
	container, containerErr := client.LoadContainer(ctx, name)
	if containerErr == nil {
		task, err := container.Task(ctx, nil)
		if err != nil {
			log.Printf("[Delete] unable to find task for container: %s\n", name)
			return nil
		}

		log.Printf("[Delete] removing CNI network for: %s\n", task.ID())

		id := NetID(task)
		netns := NetNamespace(task)

		if err := cni.Remove(ctx, id, netns); err != nil {
			return errors.Wrapf(err, "Failed to remove network for task: %q, %v", id, err)
		}
		log.Printf("[Delete] removed: %s from namespace: %s, ID: %s\n", name, netns, id)

		return nil
	}

	return errors.Wrapf(containerErr, "Unable to find container: %s, error: %s", name, containerErr)
}

// GetIPAddress returns the IP address of the created container
func GetIPAddress(result *gocni.CNIResult, task containerd.Task) (net.IP, error) {
	// Get the IP of the created interface
	var ip net.IP
	for ifName, config := range result.Interfaces {
		if config.Sandbox == NetNamespace(task) {
			for _, ipConfig := range config.IPConfigs {
				if ifName != "lo" && ipConfig.IP.To4() != nil {
					ip = ipConfig.IP
				}
			}
		}
	}
	if ip == nil {
		return nil, fmt.Errorf("unable to get IP address for: %s", task.ID())
	}
	return ip, nil
}

func NetID(task containerd.Task) string {
	return fmt.Sprintf("%s-%d", task.ID(), task.Pid())
}

func NetNamespace(task containerd.Task) string {
	return fmt.Sprintf(NetNSPathFmt, task.Pid())
}

func dirEmpty(dirname string) (isEmpty bool) {
	if !dirExists(dirname) {
		return
	}

	f, err := os.Open(dirname)
	if err != nil {
		return
	}
	defer func() { _ = f.Close() }()

	// If the first file is EOF, the directory is empty
	if _, err = f.Readdir(1); err == io.EOF {
		isEmpty = true
	}
	return isEmpty
}

func dirExists(dirname string) bool {
	exists, info := pathExists(dirname)
	if !exists {
		return false
	}

	return info.IsDir()
}

func pathExists(path string) (bool, os.FileInfo) {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return false, nil
	}

	return true, info
}
