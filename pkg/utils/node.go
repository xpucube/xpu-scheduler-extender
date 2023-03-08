package utils

import "k8s.io/api/core/v1"

// Is the Node for GPU sharing
func IsXPUSharesNode(node *v1.Node) bool {
	return GetXPUSharesCapacity(node) > 0
}

// Get the total XPU capacity of the node
func GetXPUSharesCapacity(node *v1.Node) int {
	val, ok := node.Status.Capacity[ResourceName]

	if !ok {
		return 0
	}

	return int(val.Value())
}

// Get the GPU count of the node
func GetGPUCountInNode(node *v1.Node) int {
	val, ok := node.Status.Capacity[CountName]

	if !ok {
		return int(0)
	}

	return int(val.Value())
}
