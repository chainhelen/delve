package native

import (
	"fmt"
	"syscall"
	"time"
	"unsafe"

	sys "golang.org/x/sys/unix"

	"github.com/go-delve/delve/pkg/proc/linutil"
)

// PtraceGetRegset returns floating point registers of the specified thread
// using PTRACE.
// See i386_linux_fetch_inferior_registers in gdb/i386-linux-nat.c.html
// and i386_supply_xsave in gdb/i386-tdep.c.html
// and Section 13.1 (and following) of Intel® 64 and IA-32 Architectures Software Developer’s Manual, Volume 1: Basic Architecture
func PtraceGetRegset(tid int) (regset linutil.I386Xstate, err error) {
	_, _, err = syscall.Syscall6(syscall.SYS_PTRACE, sys.PTRACE_GETFPREGS, uintptr(tid), uintptr(0), uintptr(unsafe.Pointer(&regset.I386PtraceFpRegs)), 0, 0)
	if err == syscall.Errno(0) || err == syscall.ENODEV {
		// ignore ENODEV, it just means this CPU doesn't have X87 registers (??)
		err = nil
	}

	var xstateargs [_X86_XSTATE_MAX_SIZE]byte
	iov := sys.Iovec{Base: &xstateargs[0], Len: _X86_XSTATE_MAX_SIZE}
	_, _, err = syscall.Syscall6(syscall.SYS_PTRACE, sys.PTRACE_GETREGSET, uintptr(tid), _NT_X86_XSTATE, uintptr(unsafe.Pointer(&iov)), 0, 0)
	if err != syscall.Errno(0) {
		if err == syscall.ENODEV || err == syscall.EIO {
			// ignore ENODEV, it just means this CPU or kernel doesn't support XSTATE, see https://github.com/go-delve/delve/issues/1022
			// also ignore EIO, it means that we are running on an old kernel (pre 2.6.34) and PTRACE_GETREGSET is not implemented
			err = nil
		}
		return
	} else {
		err = nil
	}

	regset.Xsave = xstateargs[:iov.Len]
	err = linutil.I386XstateRead(regset.Xsave, false, &regset)
	return
}

// gdb x86_linux_get_thread_area (pid_t pid, void *addr, unsigned int *base_addr)
// struct user_desc at https://golang.org/src/runtime/sys_linux_386.s
// PTRACE_GET_THREAD_AREA http://man7.org/linux/man-pages/man2/ptrace.2.html
type UserDesc struct {
	EntryNumber uint32
	BaseAddr    uint32
	Limit       uint32
	Flag        uint32
}

func PtraceGetTls(gs int32, tid int) uint32 {
	// I not sure this defination of struct UserStruct is right
	addr := uint32(0)
	ud := [4]uint32{}
	_, _, err := syscall.Syscall(sys.PTRACE_GET_THREAD_AREA, uintptr(tid), uintptr(gs-4), uintptr(unsafe.Pointer(&ud)))
	if err == syscall.Errno(0) || err == syscall.ENODEV {
		panic(err)
	}
	fmt.Printf("!!!!!!  %d %s, gs %d\n", tid, err, gs)
	fmt.Println(ud)
	fmt.Println(addr)

	for {
		time.Sleep(time.Second)
	}

	return ud[1]
}
