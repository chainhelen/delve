package proc

import (
	"github.com/go-delve/delve/pkg/dwarf/util"
	"testing"
)

func TestIssue554(t *testing.T) {
	// unsigned integer overflow in proc.(*memCache).contains was
	// causing it to always return true for address 0xffffffffffffffff
	mem := memCache{true, 0x20, make([]byte, 100), nil}
	var addr uint64
	switch util.PtrSizeByRuntimeArch() {
	case 8:
		addr = 0xffffffffffffffff
	case 4:
		addr = 0xffffffff
	}
	if mem.contains(uintptr(addr), 40) {
		t.Fatalf("should be false")
	}
}
