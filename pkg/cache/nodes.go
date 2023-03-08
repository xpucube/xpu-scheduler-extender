package cache

import (
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"

	"github.com/YoYoContainerService/xpu-scheduler-extender/pkg/utils"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const (
	OptimisticLockErrorMsg = "the object has been modified; please apply your changes to the latest version and try again"
)

// NodeInfo is node level aggregated information.
type NodeInfo struct {
	name           string
	node           *v1.Node
	devs           map[int]*DeviceInfo
	gpuCount       int
	gpuTotalMemory int
	rwmu           *sync.RWMutex
}

// Create Node Level
func NewNodeInfo(node *v1.Node) *NodeInfo {
	log.Printf("debug: node creation with new node name for %s", node.Name)

	devMap := map[int]*DeviceInfo{}
	for i := 0; i < utils.GetGPUCountInNode(node); i++ {
		// FIXME: now we assuming all devices in one node are the same
		devMap[i] = newDeviceInfo(i, uint(utils.GetXPUSharesCapacity(node)/utils.GetGPUCountInNode(node)))
	}

	if len(devMap) == 0 {
		log.Printf("warn: node [%s] with nodeinfo %v has no devices", node.Name, node)
	}

	return &NodeInfo{
		name:           node.Name,
		node:           node,
		devs:           devMap,
		gpuCount:       utils.GetGPUCountInNode(node),
		gpuTotalMemory: utils.GetXPUSharesCapacity(node),
		rwmu:           new(sync.RWMutex),
	}
}

// Only update the devices when the length of devs is 0
func (n *NodeInfo) Reset(node *v1.Node) {
	n.gpuCount = utils.GetGPUCountInNode(node)
	n.gpuTotalMemory = utils.GetXPUSharesCapacity(node)
	n.node = node
	if n.gpuCount == 0 {
		log.Printf("warn: reset for node [%s] but the gpu count is 0", node.Name)
	}

	if n.gpuTotalMemory == 0 {
		log.Printf("warn: reset for node [%s] but the XPU shares is 0", node.Name)
	}

	if len(n.devs) == 0 && n.gpuCount > 0 {
		devMap := map[int]*DeviceInfo{}
		for i := 0; i < utils.GetGPUCountInNode(node); i++ {
			devMap[i] = newDeviceInfo(i, uint(n.gpuTotalMemory/n.gpuCount))
		}
		n.devs = devMap
	}
	log.Printf("info: node reset update information for [%s] with devs %v", node.Name, n.devs)
}

func (n *NodeInfo) GetName() string {
	return n.name
}

func (n *NodeInfo) GetDevs() []*DeviceInfo {
	devs := make([]*DeviceInfo, n.gpuCount)
	for i, dev := range n.devs {
		devs[i] = dev
	}
	return devs
}

func (n *NodeInfo) GetNode() *v1.Node {
	return n.node
}

func (n *NodeInfo) GetNodeTotalGPUMemory() int {
	return n.gpuTotalMemory
}

func (n *NodeInfo) GetGPUCount() int {
	return n.gpuCount
}

func (n *NodeInfo) removePod(pod *v1.Pod) {
	n.rwmu.Lock()
	defer n.rwmu.Unlock()

	id := utils.GetGPUIDFromAnnotation(pod)
	if id >= 0 {
		dev, found := n.devs[id]
		if !found {
			log.Printf("warn: pod [%s] in namespace [%s] failed to find the GPU[%d] in node [%s]", pod.Name, pod.Namespace, id, n.name)
		} else {
			dev.removePod(pod)
		}
	} else {
		log.Printf("warn: pod [%s] in namespace [%s] is not set the GPU[%d] in node [%s]", pod.Name, pod.Namespace, id, n.name)
	}
}

// Add the Pod which has the GPU id to the node
func (n *NodeInfo) addOrUpdatePod(pod *v1.Pod) (added bool) {
	n.rwmu.Lock()
	defer n.rwmu.Unlock()

	id := utils.GetGPUIDFromAnnotation(pod)
	log.Printf("debug: pod [%s] in namespace [%s] with the GPU[%d] should be added to device map",
		pod.Name,
		pod.Namespace,
		id)
	if id >= 0 {
		dev, found := n.devs[id]
		if !found {
			log.Printf("warn: pod [%s] in namespace [%s] failed to find the GPU[%d] in node [%s]", pod.Name, pod.Namespace, id, n.name)
		} else {
			dev.addPod(pod)
			added = true
		}
	} else {
		log.Printf("warn: pod [%s] in namespace [%s] is not set the GPU[%d] in node [%s]", pod.Name, pod.Namespace, id, n.name)
	}
	return added
}

// check if the pod can be allocated on the node
func (n *NodeInfo) Assume(pod *v1.Pod) (allocatable bool) {
	allocatable = false

	n.rwmu.RLock()
	defer n.rwmu.RUnlock()

	availableXPUs := n.getAvailableXPUs()
	reqXPUShares  := uint(utils.GetRequestXPUSharesFromPodResource(pod))
	log.Printf("debug: all XPU Shares on this node: %v in node %s", availableXPUs, n.name)

	if len(availableXPUs) > 0 {
		for devID := 0; devID < len(n.devs); devID++ {
			availableXPU, ok := availableXPUs[devID]
			if ok {
				if availableXPU >= reqXPUShares {
					allocatable = true
					break
				}
			}
		}
	}

	return allocatable

}

func (n *NodeInfo) Allocate(clientset *kubernetes.Clientset, pod *v1.Pod) (err error) {
	var newPod *v1.Pod
	n.rwmu.Lock()
	defer n.rwmu.Unlock()
	log.Printf("info: beginning to allocate XPU shares for pod [%s] in namespace [%s]", pod.Name, pod.Namespace)
	// 1. update the pod spec
	devId, found := n.allocateGPUID(pod)
	if found {
		log.Printf("info: GPU[%d] wil be allocated to pod [%s] in namespace [%s]", devId, pod.Name, pod.Namespace)
		newPod = utils.GetUpdatedPodAnnotationSpec(pod, devId, n.GetNodeTotalGPUMemory()/n.GetGPUCount())
		_, err = clientset.CoreV1().Pods(newPod.Namespace).Update(newPod)
		if err != nil {
			// the object has been modified; please apply your changes to the latest version and try again
			if err.Error() == OptimisticLockErrorMsg {
				// retry
				pod, err = clientset.CoreV1().Pods(pod.Namespace).Get(pod.Name, metav1.GetOptions{})
				if err != nil {
					return err
				}
				newPod = utils.GetUpdatedPodAnnotationSpec(pod, devId, n.GetNodeTotalGPUMemory()/n.GetGPUCount())
				_, err = clientset.CoreV1().Pods(newPod.Namespace).Update(newPod)
				if err != nil {
					return err
				}
			} else {
				return err
			}
		}
	} else {
		err = fmt.Errorf("the node %s can't place the pod [%s] in namespace [%s]", pod.Spec.NodeName, pod.Name, pod.Namespace)
	}

	// 2. bind the pod to the node
	if err == nil {
		binding := &v1.Binding{
			ObjectMeta: metav1.ObjectMeta{Name: pod.Name, UID: pod.UID},
			Target:     v1.ObjectReference{Kind: "Node", Name: n.name},
		}
		log.Printf("info: trying to bind pod [%s] in [%s] namespace to node [%s]",
			pod.Name,
			pod.Namespace,
			pod.Spec.NodeName)
		err = clientset.CoreV1().Pods(pod.Namespace).Bind(binding)
		if err != nil {
			log.Printf("warn: failed to bind the pod [%s] in namespace [%s] due to %v", pod.Name, pod.Namespace, err)
			return err
		}
	}

	// 3. update the device info if the pod is update successfully
	if err == nil {
		log.Printf("info: trying to add pod [%s] in namespace [%s] to dev [%d]",
			pod.Name,
			pod.Namespace,
			devId)
		dev, found := n.devs[devId]
		if !found {
			log.Printf("warn: pod [%s] in namespace [%s] failed to find the GPU[%d] in node [%s]", pod.Name, pod.Namespace, devId, n.name)
		} else {
			dev.addPod(newPod)
		}
	}
	log.Printf("info: ending to allocate XPU shares for pod [%s] in namespace [%s]", pod.Name, pod.Namespace)
	return err
}

// allocate the GPU ID to the pod
func (n *NodeInfo) allocateGPUID(pod *v1.Pod) (candidateDevID int, found bool) {

	reqShares      := uint(0)
	found          = false
	candidateDevID = -1
	candidateXPUShares := uint(0)
	availableXPUShares := n.getAvailableXPUs()
	availableXPUCount  := uint(0)
	allocatedXPUShares := map[int]uint{}

	reqShares = uint(utils.GetRequestXPUSharesFromPodResource(pod))

	if reqShares > uint(0) {
		log.Printf("info: request XPU shares for pod [%s] in namespace [%s]: [%d]", pod.Name, pod.Namespace, reqShares)
		log.Printf("info: available XPU shares: %v in node [%s]", availableXPUShares, n.name)
		if len(availableXPUShares) > 0 {
			for devID := 0; devID < len(n.devs); devID++ {
				availableShares, ok := availableXPUShares[devID]
				availableXPUCount += availableShares
				if ok {
					if availableShares >= reqShares {
						if candidateDevID == -1 || candidateXPUShares > availableShares {
							candidateDevID = devID
							candidateXPUShares = availableShares
						}
						// first we found one device is enough for request
						found = true
						log.Printf("info: find candidate GPU[%d] for pod [%s] in namespace [%s] successfully.",
							candidateDevID,
							pod.Name,
							pod.Namespace)
					}
				}
			}
			if !found {
				// FIMXE: should separate shares to different devices
				if (availableXPUCount >= reqShares) {
					for devID := 0; devID < len(n.devs); devID++ {
						availableShares, ok := availableXPUShares[devID]
						if ok {
							if (availableXPUCount > 0) {
								allocatedXPUShares[devID] = availableShares;
							}
						}
					}
				}
			}
		}
		
		
		if !found  {
			log.Printf("warn: failed to find available XPU shares [%d] for the pod [%s] in the namespace [%s]",
				reqShares,
				pod.Name,
				pod.Namespace)
		}
	}

	return candidateDevID, found
}

func (n *NodeInfo) getAvailableXPUs() (availableXPUShares map[int]uint) {
	allXPUShares       := n.getAllXPUs()
	usedXPUShares      := n.getUsedXPUs()
	unhealthyXPUShares := n.getUnhealthyXPUs()
	availableXPUShares  = map[int]uint{}
	for id, totalShares := range allXPUShares {
		if usedShares, found := usedXPUShares[id]; found {
			availableXPUShares[id] = totalShares - usedShares
		}
	}
	log.Printf("info: available XPU shares list %v before removing unhealty XPU shares", availableXPUShares)
	for id, _ := range unhealthyXPUShares {
		log.Printf("info: delete dev %d from availble XPU shares list", id)
		delete(availableXPUShares, id)
	}
	log.Printf("info: available XPU shares list %v after removing unhealty XPU shares", availableXPUShares)

	return availableXPUShares
}

// device index: XPU shares
func (n *NodeInfo) getUsedXPUs() (usedXPUShares map[int]uint) {
	usedXPUShares = map[int]uint{}
	for _, dev := range n.devs {
		usedXPUShares[dev.idx] = dev.GetDevUsedXPUShares()
	}
	log.Printf("info: used XPU shares: %v in node [%s], and devs %v", usedXPUShares, n.name, n.devs)
	return usedXPUShares
}

// device index: XPU shares
func (n *NodeInfo) getAllXPUs() (allXPUShares map[int]uint) {
	allXPUShares = map[int]uint{}
	for _, dev := range n.devs {
		allXPUShares[dev.idx] = dev.totalXPUShares
	}
	log.Printf("info: all XPU shares: %v in node [%s], and dev %v", allXPUShares, n.name, n.devs)
	return allXPUShares
}

// getUnhealthyXPUs get the unhealthy GPUs from configmap
func (n *NodeInfo) getUnhealthyXPUs() (unhealthyGPUs map[int]bool) {
	unhealthyGPUs = map[int]bool{}
	name := fmt.Sprintf("unhealthy-gpu-[%s]", n.GetName())
	log.Printf("info: try to find unhealthy node [%s]", name)
	cm := getConfigMap(name)
	if cm == nil {
		return
	}

	if devicesStr, found := cm.Data["gpus"]; found {
		log.Printf("warn: the unhelathy gpus [%s]", devicesStr)
		idsStr := strings.Split(devicesStr, ",")
		for _, sid := range idsStr {
			id, err := strconv.Atoi(sid)
			if err != nil {
				log.Printf("warn: failed to parse id [%s] due to %v", sid, err)
			}
			unhealthyGPUs[id] = true
		}
	} else {
		log.Println("info: skip, because there are no unhealthy gpus")
	}

	return

}
