package utils

import (
	"fmt"
	"log"
	"strconv"
	"time"

	"k8s.io/api/core/v1"
)

// AssignedNonTerminatedPod selects pods that are assigned and non-terminal (scheduled and running).
func AssignedNonTerminatedPod(pod *v1.Pod) bool {
	if pod.DeletionTimestamp != nil {
		return false
	}

	if len(pod.Spec.NodeName) == 0 {
		return false
	}
	if pod.Status.Phase == v1.PodSucceeded || pod.Status.Phase == v1.PodFailed {
		return false
	}
	return true
}

// IsCompletePod determines if the pod is complete
func IsCompletePod(pod *v1.Pod) bool {
	if pod.DeletionTimestamp != nil {
		return true
	}

	if pod.Status.Phase == v1.PodSucceeded || pod.Status.Phase == v1.PodFailed {
		return true
	}
	return false
}

// IsGPUsharingPod determines if it's the pod for GPU sharing
func IsGPUsharingPod(pod *v1.Pod) bool {
	return GetRequestXPUSharesFromPodResource(pod) > 0
}

// GetGPUIDFromAnnotation gets GPU ID from Annotation
func GetGPUIDFromAnnotation(pod *v1.Pod) int {
	id := -1
	if len(pod.ObjectMeta.Annotations) > 0 {
		value, found := pod.ObjectMeta.Annotations[EnvResourceIndex]
		if found {
			var err error
			id, err = strconv.Atoi(value)
			if err != nil {
				log.Printf("warn: failed due to %v for pod %s in namespace %s", err, pod.Name, pod.Namespace)
				id = -1
			}
		}
	}

	return id
}

// GetGPUIDFromEnv gets GPU ID from Env
func GetGPUIDFromEnv(pod *v1.Pod) int {
	id := -1
	for _, container := range pod.Spec.Containers {
		id = getGPUIDFromContainer(container)
		if id >= 0 {
			return id
		}
	}

	return id
}

func getGPUIDFromContainer(container v1.Container) (devIdx int) {
	devIdx = -1
	var err error
loop:
	for _, env := range container.Env {
		if env.Name == EnvResourceIndex {
			devIdx, err = strconv.Atoi(env.Value)
			if err != nil {
				log.Printf("warn: failed due to %v for %s", err, container.Name)
				devIdx = -1
			}
			break loop
		}
	}

	return devIdx
}

// GetXPUSharesFromPodAnnotation gets the XPU shares of the pod
func GetXPUSharesFromPodAnnotation(pod *v1.Pod) (xpuShares uint) {
	if len(pod.ObjectMeta.Annotations) > 0 {
		value, found := pod.ObjectMeta.Annotations[EnvResourceByPod]
		if found {
			s, _ := strconv.Atoi(value)
			if s < 0 {
				s = 0
			}

			xpuShares += uint(s)
		}
	}

	log.Printf("debug: pod %s in namespace %s with status %v has XPU shares %d",
		pod.Name,
		pod.Namespace,
		pod.Status.Phase,
		xpuShares)
	return xpuShares
}

// GetXPUSharesFromPodEnv gets the GPU Memory of the pod
func GetXPUSharesFromPodEnv(pod *v1.Pod) (xpuShares uint) {
	for _, container := range pod.Spec.Containers {
		xpuShares += getXPUSharesFromContainerEnv(container)
	}
	log.Printf("debug: pod %s in namespace %s with status %v has XPU shares %d",
		pod.Name,
		pod.Namespace,
		pod.Status.Phase,
		xpuShares)
	return xpuShares
}

func getXPUSharesFromContainerEnv(container v1.Container) (xpuShares uint) {
	xpuShares = 0
loop:
	for _, env := range container.Env {
		if env.Name == EnvResourceByPod {
			s, _ := strconv.Atoi(env.Value)
			if s < 0 {
				s = 0
			}
			xpuShares = uint(s)
			break loop
		}
	}

	return xpuShares
}

// GetRequestXPUSharesFromPodResource gets XPU shares of the Pod
func GetRequestXPUSharesFromPodResource(pod *v1.Pod) int {
	var total int
	containers := pod.Spec.Containers
	for _, container := range containers {
		if val, ok := container.Resources.Limits[ResourceName]; ok {
			total += int(val.Value())
		}
	}
	return total
}

// GetRequestXPUSharesFromContainerResource gets GPU Memory of the Container
func GetRequestXPUSharesFromContainerResource(container v1.Container) int {
	var total int
	if val, ok := container.Resources.Limits[ResourceName]; ok {
		total += int(val.Value())
	}
	return total
}

// GetUpdatedPodEnvSpec updates pod env with devId
func GetUpdatedPodEnvSpec(oldPod *v1.Pod, devId int, totalXPUSharesByDev int) (newPod *v1.Pod) {
	newPod = oldPod.DeepCopy()
	for i, c := range newPod.Spec.Containers {
		xpuShares := GetRequestXPUSharesFromContainerResource(c)

		if xpuShares > 0 {
			envs := []v1.EnvVar{
				v1.EnvVar{Name: EnvResourceIndex, Value: fmt.Sprintf("%d", devId)},
				v1.EnvVar{Name: EnvResourceByPod, Value: fmt.Sprintf("%d", xpuShares)},
				v1.EnvVar{Name: EnvResourceByDev, Value: fmt.Sprintf("%d", totalXPUSharesByDev)},
				v1.EnvVar{Name: EnvAssignedFlag, Value: "false"},
			}

			for _, env := range envs {
				newPod.Spec.Containers[i].Env = append(newPod.Spec.Containers[i].Env, env)
			}
		}
	}

	return newPod
}

// GetUpdatedPodAnnotationSpec updates pod env with devId
func GetUpdatedPodAnnotationSpec(oldPod *v1.Pod, devId int, totalXPUSharesByDev int) (newPod *v1.Pod) {
	newPod = oldPod.DeepCopy()
	if len(newPod.ObjectMeta.Annotations) == 0 {
		newPod.ObjectMeta.Annotations = map[string]string{}
	}

	now := time.Now()
	newPod.ObjectMeta.Annotations[EnvResourceIndex]      = fmt.Sprintf("%d", devId)
	newPod.ObjectMeta.Annotations[EnvResourceByDev]      = fmt.Sprintf("%d", totalXPUSharesByDev)
	newPod.ObjectMeta.Annotations[EnvResourceByPod]      = fmt.Sprintf("%d", GetRequestXPUSharesFromPodResource(newPod))
	newPod.ObjectMeta.Annotations[EnvAssignedFlag]       = "false"
	newPod.ObjectMeta.Annotations[EnvResourceAssumeTime] = fmt.Sprintf("%d", now.UnixNano())

	return newPod
}
