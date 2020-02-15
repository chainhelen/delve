package util

import (
	"fmt"
	"runtime"
)

func PtrSizeByRuntimeArch() int {
	// TODO: find better way to determine proc arch (perhaps use executable file info).
	switch runtime.GOARCH {
	case "386":
		return 4
	case "amd64":
		return 8
	case "arm64":
		return 8
	}
	panic(fmt.Errorf("not support arch %s", runtime.GOARCH))
}
