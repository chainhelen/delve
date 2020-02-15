package native

import (
	"syscall"
	"unsafe"

	sys "golang.org/x/sys/unix"

	"github.com/go-delve/delve/pkg/proc"
	"github.com/go-delve/delve/pkg/proc/linutil"
)

func (t *Thread) restoreRegisters(savedRegs proc.Registers) error {
	sr := savedRegs.(*linutil.I386Registers)

	var restoreRegistersErr error
	t.dbp.execPtraceFunc(func() {
		oldRegs := (*sys.PtraceRegs)(sr.Regs)

		var currentRegs sys.PtraceRegs
		restoreRegistersErr = sys.PtraceGetRegs(t.ID, &currentRegs)
		if restoreRegistersErr != nil {
			return
		}
		// restoreRegisters is only supposed to restore CPU registers, not XFS and XGS
		oldRegs.Xfs = currentRegs.Xfs
		oldRegs.Xgs = currentRegs.Xgs

		restoreRegistersErr = sys.PtraceSetRegs(t.ID, oldRegs)

		if restoreRegistersErr != nil {
			return
		}
		if sr.Fpregset.Xsave != nil {
			iov := sys.Iovec{Base: &sr.Fpregset.Xsave[0], Len: uint32(len(sr.Fpregset.Xsave))}
			_, _, restoreRegistersErr = syscall.Syscall6(syscall.SYS_PTRACE, sys.PTRACE_SETREGSET, uintptr(t.ID), _NT_X86_XSTATE, uintptr(unsafe.Pointer(&iov)), 0, 0)
			return
		}

		_, _, restoreRegistersErr = syscall.Syscall6(syscall.SYS_PTRACE, sys.PTRACE_SETFPREGS, uintptr(t.ID), uintptr(0), uintptr(unsafe.Pointer(&sr.Fpregset.I386PtraceFpRegs)), 0, 0)
		return
	})
	if restoreRegistersErr == syscall.Errno(0) {
		restoreRegistersErr = nil
	}
	return restoreRegistersErr
}
