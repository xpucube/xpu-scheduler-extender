package cache

import (
	"log"
	"sync"

	"github.com/YoYoContainerService/xpu-scheduler-extender/pkg/utils"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

type DeviceInfo struct {
	idx		int
	podMap		map[types.UID]*v1.Pod
	totalXPUShares	uint
	rwmu		*sync.RWMutex
}

func (d *DeviceInfo) GetPods() []*v1.Pod {
	pods := []*v1.Pod{}
	for _, pod := range d.podMap {
		pods = append(pods, pod)
	}
	return pods
}

func newDeviceInfo(index int, totalXPUShares uint) *DeviceInfo {
	return &DeviceInfo{
		idx:		index,
		totalXPUShares:	totalXPUShares,
		podMap:		map[types.UID]*v1.Pod{},
		rwmu:		new(sync.RWMutex),
	}
}

func (d *DeviceInfo) GetDevTotalXPUShares() uint {
	return d.totalXPUShares
}

func (d *DeviceInfo) GetDevUsedXPUShares() (gpuMem uint) {
	//log.Printf("debug: devices pod map %v, and its address is %p", d.podMap, d)
	d.rwmu.RLock()
	defer d.rwmu.RUnlock()
	for _, pod := range d.podMap {
		if pod.Status.Phase == v1.PodSucceeded || pod.Status.Phase == v1.PodFailed {
			log.Printf("debug: skip the pod [%s] in namespace [%s] due to its status is [%s]", pod.Name, pod.Namespace, pod.Status.Phase)
			continue
		}
		gpuMem += utils.GetXPUSharesFromPodAnnotation(pod)
	}
	return gpuMem
}

func (d *DeviceInfo) addPod(pod *v1.Pod) {
	log.Printf("debug: add pod [%s] in namespace [%s] with the GPU[%d] will be added to device map",
		pod.Name,
		pod.Namespace,
		d.idx)
	d.rwmu.Lock()
	defer d.rwmu.Unlock()
	d.podMap[pod.UID] = pod
	//log.Printf("debug: add pod after updated is %v, and its address is %p", d.podMap, d)
}

func (d *DeviceInfo) removePod(pod *v1.Pod) {
	log.Printf("debug: remove pod [%s] in namespace [%s] with the GPU[%d] will be removed from device map",
		pod.Name,
		pod.Namespace,
		d.idx)
	d.rwmu.Lock()
	defer d.rwmu.Unlock()
	delete(d.podMap, pod.UID)
	//log.Printf("debug: remove pod after updated is %v, and its address is %p", d.podMap, d)
}
