package proc

import (
	"encoding/binary"
	"github.com/go-delve/delve/pkg/dwarf/frame"
	"github.com/go-delve/delve/pkg/dwarf/op"
	"golang.org/x/arch/x86/x86asm"
	"strings"
)

// I386 represents the I386 CPU architecture.
type I386 struct {
	gStructOffset uint64
	goos          string

	// crosscall2fn is the DIE of crosscall2, a function used by the go runtime
	// to call C functions. This function in go 1.9 (and previous versions) had
	// a bad frame descriptor which needs to be fixed to generate good stack
	// traces.
	crosscall2fn *Function

	// sigreturnfn is the DIE of runtime.sigreturn, the return trampoline for
	// the signal handler. See comment in FixFrameUnwindContext for a
	// description of why this is needed.
	sigreturnfn *Function
}

const (
	i386DwarfIPRegNum uint64 = 8
	i386DwarfSPRegNum uint64 = 4
	i386DwarfBPRegNum uint64 = 5
)

var i386BreakInstruction = []byte{0xCC}

// I386Arch returns an initialized I386Arch
// struct.
func I386Arch(goos string) *I386 {
	return &I386{
		goos: goos,
	}
}

// PtrSize returns the size of a pointer
// on this architecture.
func (i *I386) PtrSize() int {
	return 4
}

// MaxInstructionLength returns the maximum lenght of an instruction.
func (i *I386) MaxInstructionLength() int {
	return 15
}

// BreakpointInstruction returns the Breakpoint
// instruction for this architecture.
func (i *I386) BreakpointInstruction() []byte {
	return i386BreakInstruction
}

// BreakInstrMovesPC returns whether the
// breakpoint instruction will change the value
// of PC after being executed
func (i *I386) BreakInstrMovesPC() bool {
	return true
}

// BreakpointSize returns the size of the
// breakpoint instruction on this architecture.
func (i *I386) BreakpointSize() int {
	return len(i386BreakInstruction)
}

// TODO, Not sure, always return false for now. Need to test on windows.
func (i *I386) DerefTLS() bool {
	return false
}

// FixFrameUnwindContext adds default architecture rules to fctxt or returns
// the default frame unwind context if fctxt is nil.
func (i *I386) FixFrameUnwindContext(fctxt *frame.FrameContext, pc uint64, bi *BinaryInfo) *frame.FrameContext {
	if i.sigreturnfn == nil {
		i.sigreturnfn = bi.LookupFunc["runtime.sigreturn"]
	}

	if fctxt == nil || (i.sigreturnfn != nil && pc >= i.sigreturnfn.Entry && pc < i.sigreturnfn.End) {
		// When there's no frame descriptor entry use BP (the frame pointer) instead
		// - return register is [bp + i.PtrSize()] (i.e. [cfa-i.PtrSize()])
		// - cfa is bp + i.PtrSize()*2
		// - bp is [bp] (i.e. [cfa-i.PtrSize()*2])
		// - sp is cfa

		// When the signal handler runs it will move the execution to the signal
		// handling stack (installed using the sigaltstack system call).
		// This isn't i proper stack switch: the pointer to g in TLS will still
		// refer to whatever g was executing on that thread before the signal was
		// received.
		// Since go did not execute i stack switch the previous value of sp, pc
		// and bp is not saved inside g.sched, as it normally would.
		// The only way to recover is to either read sp/pc from the signal context
		// parameter (the ucontext_t* parameter) or to unconditionally follow the
		// frame pointer when we get to runtime.sigreturn (which is what we do
		// here).

		return &frame.FrameContext{
			RetAddrReg: i386DwarfIPRegNum,
			Regs: map[uint64]frame.DWRule{
				i386DwarfIPRegNum: frame.DWRule{
					Rule:   frame.RuleOffset,
					Offset: int64(-i.PtrSize()),
				},
				i386DwarfBPRegNum: frame.DWRule{
					Rule:   frame.RuleOffset,
					Offset: int64(-2 * i.PtrSize()),
				},
				i386DwarfSPRegNum: frame.DWRule{
					Rule:   frame.RuleValOffset,
					Offset: 0,
				},
			},
			CFA: frame.DWRule{
				Rule:   frame.RuleCFA,
				Reg:    i386DwarfBPRegNum,
				Offset: int64(2 * i.PtrSize()),
			},
		}
	}

	if i.crosscall2fn == nil {
		i.crosscall2fn = bi.LookupFunc["crosscall2"]
	}

	if i.crosscall2fn != nil && pc >= i.crosscall2fn.Entry && pc < i.crosscall2fn.End {
		rule := fctxt.CFA
		if rule.Offset == crosscall2SPOffsetBad {
			switch i.goos {
			case "windows":
				rule.Offset += crosscall2SPOffsetWindows
			default:
				rule.Offset += crosscall2SPOffsetNonWindows
			}
		}
		fctxt.CFA = rule
	}

	// We assume that EBP is the frame pointer and we want to keep it updated,
	// so that we can use it to unwind the stack even when we encounter frames
	// without descriptor entries.
	// If there isn't i rule already we emit one.
	if fctxt.Regs[i386DwarfBPRegNum].Rule == frame.RuleUndefined {
		fctxt.Regs[i386DwarfBPRegNum] = frame.DWRule{
			Rule:   frame.RuleFramePointer,
			Reg:    i386DwarfBPRegNum,
			Offset: 0,
		}
	}

	return fctxt
}

// cgocallSPOffsetSaveSlot is the offset from systemstack.SP where
// (goroutine.SP - StackHi) is saved in runtime.asmcgocall after the stack
// switch happens.
const i386cgocallSPOffsetSaveSlot = 0x28

// SwitchStack will use the current frame to determine if it's time to
// switch between the system stack and the goroutine stack or vice versa.
// Sets it.atend when the top of the stack is reached.
func (i *I386) SwitchStack(it *stackIterator, _ *op.DwarfRegisters) bool {
	if it.frame.Current.Fn == nil {
		return false
	}
	switch it.frame.Current.Fn.Name {
	case "runtime.asmcgocall":
		if it.top || !it.systemstack {
			return false
		}

		// This function is called by a goroutine to execute a C function and
		// switches from the goroutine stack to the system stack.
		// Since we are unwinding the stack from callee to caller we have to switch
		// from the system stack to the goroutine stack.
		off, _ := readIntRaw(it.mem, uintptr(it.regs.SP()+i386cgocallSPOffsetSaveSlot), int64(it.bi.Arch.PtrSize())) // reads "offset of SP from StackHi" from where runtime.asmcgocall saved it
		oldsp := it.regs.SP()
		it.regs.Reg(it.regs.SPRegNum).Uint64Val = uint64(int64(it.stackhi) - off)

		// runtime.asmcgocall can also be called from inside the system stack,
		// in that case no stack switch actually happens
		if it.regs.SP() == oldsp {
			return false
		}
		it.systemstack = false

		// advances to the next frame in the call stack
		it.frame.addrret = uint64(int64(it.regs.SP()) + int64(it.bi.Arch.PtrSize()))
		it.frame.Ret, _ = readUintRaw(it.mem, uintptr(it.frame.addrret), int64(it.bi.Arch.PtrSize()))
		it.pc = it.frame.Ret

		it.top = false
		return true

	case "runtime.cgocallback_gofunc":
		// For a detailed description of how this works read the long comment at
		// the start of $GOROOT/src/runtime/cgocall.go and the source code of
		// runtime.cgocallback_gofunc in $GOROOT/src/runtime/asm_amd64.s
		//
		// When a C functions calls back into go it will eventually call into
		// runtime.cgocallback_gofunc which is the function that does the stack
		// switch from the system stack back into the goroutine stack
		// Since we are going backwards on the stack here we see the transition
		// as goroutine stack -> system stack.

		if it.top || it.systemstack {
			return false
		}

		if it.g0_sched_sp <= 0 {
			return false
		}
		// entering the system stack
		it.regs.Reg(it.regs.SPRegNum).Uint64Val = it.g0_sched_sp
		// reads the previous value of g0.sched.sp that runtime.cgocallback_gofunc saved on the stack
		it.g0_sched_sp, _ = readUintRaw(it.mem, uintptr(it.regs.SP()), int64(it.bi.Arch.PtrSize()))
		it.top = false
		callFrameRegs, ret, retaddr := it.advanceRegs()
		frameOnSystemStack := it.newStackframe(ret, retaddr)
		it.pc = frameOnSystemStack.Ret
		it.regs = callFrameRegs
		it.systemstack = true
		return true

	case "runtime.goexit", "runtime.rt0_go", "runtime.mcall":
		// Look for "top of stack" functions.
		it.atend = true
		return true

	case "runtime.mstart":
		// Calls to runtime.systemstack will switch to the systemstack then:
		// 1. alter the goroutine stack so that it looks like systemstack_switch
		//    was called
		// 2. alter the system stack so that it looks like the bottom-most frame
		//    belongs to runtime.mstart
		// If we find a runtime.mstart frame on the system stack of a goroutine
		// parked on runtime.systemstack_switch we assume runtime.systemstack was
		// called and continue tracing from the parked position.

		if it.top || !it.systemstack || it.g == nil {
			return false
		}
		if fn := it.bi.PCToFunc(it.g.PC); fn == nil || fn.Name != "runtime.systemstack_switch" {
			return false
		}

		it.switchToGoroutineStack()
		return true

	default:
		if it.systemstack && it.top && it.g != nil && strings.HasPrefix(it.frame.Current.Fn.Name, "runtime.") && it.frame.Current.Fn.Name != "runtime.fatalthrow" {
			// The runtime switches to the system stack in multiple places.
			// This usually happens through a call to runtime.systemstack but there
			// are functions that switch to the system stack manually (for example
			// runtime.morestack).
			// Since we are only interested in printing the system stack for cgo
			// calls we switch directly to the goroutine stack if we detect that the
			// function at the top of the stack is a runtime function.
			//
			// The function "runtime.fatalthrow" is deliberately excluded from this
			// because it can end up in the stack during a cgo call and switching to
			// the goroutine stack will exclude all the C functions from the stack
			// trace.
			it.switchToGoroutineStack()
			return true
		}

		return false
	}
}

func (i *I386) RegSize(regnum uint64) int {
	// XMM registers
	if regnum >= 21 && regnum <= 36 {
		return 16
	}
	// x87 registers
	if regnum >= 11 && regnum <= 18 {
		return 10
	}
	return 4
}

var i386DwarfToHardware = map[int]x86asm.Reg{
	0: x86asm.EAX,
	1: x86asm.ECX,
	2: x86asm.EDX,
	3: x86asm.EBX,
	6: x86asm.ESI,
	7: x86asm.EDI,
}

var i386DwarfToName = map[int]string{
	9:  "Eflags",
	11: "ST(0)",
	12: "ST(1)",
	13: "ST(2)",
	14: "ST(3)",
	15: "ST(4)",
	16: "ST(5)",
	17: "ST(6)",
	18: "ST(7)",
	21: "XMM0",
	22: "XMM1",
	23: "XMM2",
	24: "XMM3",
	25: "XMM4",
	26: "XMM5",
	27: "XMM6",
	28: "XMM7",
	40: "Es",
	41: "Cs",
	42: "Ss",
	43: "Ds",
	44: "Fs",
	45: "Gs",
}

func maxI386DwarfRegister() int {
	max := int(i386DwarfIPRegNum)
	for i := range i386DwarfToHardware {
		if i > max {
			max = i
		}
	}
	for i := range i386DwarfToName {
		if i > max {
			max = i
		}
	}
	return max
}

func (i *I386) RegistersToDwarfRegisters(staticBase uint64, regs Registers) op.DwarfRegisters {
	dregs := make([]*op.DwarfRegister, maxI386DwarfRegister()+1)

	dregs[i386DwarfIPRegNum] = op.DwarfRegisterFromUint64(regs.PC())
	dregs[i386DwarfSPRegNum] = op.DwarfRegisterFromUint64(regs.SP())
	dregs[amd64DwarfBPRegNum] = op.DwarfRegisterFromUint64(regs.BP())

	for dwarfReg, asmReg := range i386DwarfToHardware {
		v, err := regs.Get(int(asmReg))
		if err == nil {
			dregs[dwarfReg] = op.DwarfRegisterFromUint64(v)
		}
	}

	for _, reg := range regs.Slice(true) {
		for dwarfReg, regName := range i386DwarfToName {
			if regName == reg.Name {
				dregs[dwarfReg] = op.DwarfRegisterFromBytes(reg.Bytes)
			}
		}
	}

	return op.DwarfRegisters{
		StaticBase: staticBase,
		Regs:       dregs,
		ByteOrder:  binary.LittleEndian,
		PCRegNum:   i386DwarfIPRegNum,
		SPRegNum:   i386DwarfSPRegNum,
		BPRegNum:   i386DwarfBPRegNum,
	}
}

// AddrAndStackRegsToDwarfRegisters returns DWARF registers from the passed in
// PC, SP, and BP registers in the format used by the DWARF expression interpreter.
func (i *I386) AddrAndStackRegsToDwarfRegisters(staticBase, pc, sp, bp, lr uint64) op.DwarfRegisters {
	dregs := make([]*op.DwarfRegister, i386DwarfIPRegNum+1)
	dregs[i386DwarfIPRegNum] = op.DwarfRegisterFromUint64(pc)
	dregs[i386DwarfSPRegNum] = op.DwarfRegisterFromUint64(sp)
	dregs[i386DwarfBPRegNum] = op.DwarfRegisterFromUint64(bp)

	return op.DwarfRegisters{
		StaticBase: staticBase,
		Regs:       dregs,
		ByteOrder:  binary.LittleEndian,
		PCRegNum:   i386DwarfIPRegNum,
		SPRegNum:   i386DwarfSPRegNum,
		BPRegNum:   i386DwarfBPRegNum,
	}
}
