package scheduler

import (
	"fmt"
	"log"

	"github.com/YoYoContainerService/xpu-scheduler-extender/pkg/cache"
	"github.com/YoYoContainerService/xpu-scheduler-extender/pkg/utils"
	"k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
)

func NewXPUPredicate(clientset *kubernetes.Clientset, c *cache.SchedulerCache) *Predicate {
	return &Predicate{
		Name: "xpusharesfilter",
		Func: func(pod *v1.Pod, nodeName string, c *cache.SchedulerCache) (bool, error) {
			log.Printf("info: check if the pod name %s can be scheduled on node %s", pod.Name, nodeName)
			nodeInfo, err := c.GetNodeInfo(nodeName)
			if err != nil {
				return false, err
			}

			if !utils.IsXPUSharesNode(nodeInfo.GetNode()) {
				return false, fmt.Errorf("the node %s is not for XPU shares, need skip", nodeName)
			}

			allocatable := nodeInfo.Assume(pod)
			if !allocatable {
				return false, fmt.Errorf("insufficient XPU shares in one device")
			} else {
				log.Printf("info: the pod %s in the namespace %s can be scheduled on %s",
					pod.Name,
					pod.Namespace,
					nodeName)
			}
			return true, nil
		},
		cache: c,
	}
}
