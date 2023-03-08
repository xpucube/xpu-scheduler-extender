package utils

const (
	ResourceName = "openxpu.com/xpu-shares"
	CountName    = "openxpu.com/xpu-counts"

	EnvNVGPU              = "NVIDIA_VISIBLE_DEVICES"
	EnvResourceIndex      = "OPENXPU_XPU_SHARES_INDEX"
	EnvResourceByPod      = "OPENXPU_XPU_SHARES_POD"
	EnvResourceByDev      = "OPENXPU_XPU_SHARES_TOTAL"
	EnvAssignedFlag       = "OPENXPU_XPU_SHARES_ALLOCATED"
	EnvResourceAssumeTime = "OPENXPU_XPU_SHARES_FILTER_STAMP"
)

