package scheduler

import (
	"github.com/YoYoContainerService/xpu-scheduler-extender/pkg/cache"
	"github.com/YoYoContainerService/xpu-scheduler-extender/pkg/utils"
)

func (in Inspect) Handler(name string) *Result {
	nodes := []*Node{}
	errMsg := ""
	if len(name) == 0 {
		nodeInfos := in.cache.GetNodeinfos()
		for _, info := range nodeInfos {
			nodes = append(nodes, buildNode(info))
		}

	} else {
		node, err := in.cache.GetNodeInfo(name)
		if err != nil {
			errMsg = err.Error()
		}
		// nodeInfos = append(nodeInfos, node)
		nodes = append(nodes, buildNode(node))
	}

	return &Result{
		Nodes: nodes,
		Error: errMsg,
	}
}

func buildNode(info *cache.NodeInfo) *Node {

	devInfos := info.GetDevs()
	devs := []*Device{}
	var usedGPU uint

	for i, devInfo := range devInfos {
		dev := &Device{
			ID:       i,
			TotalGPU: devInfo.GetDevTotalXPUShares(),
			UsedGPU:  devInfo.GetDevUsedXPUShares(),
		}

		podInfos := devInfo.GetPods()
		pods := []*Pod{}
		for _, podInfo := range podInfos {
			if utils.AssignedNonTerminatedPod(podInfo) {
				pod := &Pod{
					Namespace: podInfo.Namespace,
					Name:      podInfo.Name,
					UsedGPU:   utils.GetRequestXPUSharesFromPodResource(podInfo),
				}
				pods = append(pods, pod)
			}
		}
		dev.Pods = pods
		devs = append(devs, dev)
		usedGPU += devInfo.GetDevUsedXPUShares()
	}

	return &Node{
		Name:     info.GetName(),
		TotalGPU: uint(info.GetNodeTotalGPUMemory()),
		UsedGPU:  usedGPU,
		Devices:  devs,
	}

}
