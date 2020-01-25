package op

import (
	"github.com/go-delve/delve/pkg/dwarf/util"
	"testing"
)

func TestExecuteStackProgram(t *testing.T) {
	var (
		instructions = []byte{byte(DW_OP_consts), 0x1c, byte(DW_OP_consts), 0x1c, byte(DW_OP_plus)}
		expected     = int64(56)
	)
	actual, _, err := ExecuteStackProgram(DwarfRegisters{}, instructions, util.PtrSizeByRuntimeArch())
	if err != nil {
		t.Fatal(err)
	}

	if actual != expected {
		t.Fatalf("actual %d != expected %d", actual, expected)
	}
}
