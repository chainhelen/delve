// TODO: disassembler support should be compiled in unconditionally,
// instead of being decided by the build-target architecture, and be
// part of the Arch object instead.

package proc

import (
	"golang.org/x/arch/x86/x86asm"
)

// AsmDecode decodes the assembly instruction starting at mem[0:] into asmInst.
// It assumes that the Loc and AtPC fields of asmInst have already been filled.
func (i *I386) AsmDecode(asmInst *AsmInstruction, mem []byte, regs Registers, memrw MemoryReadWriter, bi *BinaryInfo) error {
	inst, err := x86asm.Decode(mem, 64)
	if err != nil {
		asmInst.Inst = (*i386ArchInst)(nil)
		asmInst.Size = 1
		asmInst.Bytes = mem[:asmInst.Size]
		return err
	}

	asmInst.Size = inst.Len
	asmInst.Bytes = mem[:asmInst.Size]
	patchPCRelAMD64(asmInst.Loc.PC, &inst)
	asmInst.Inst = (*i386ArchInst)(&inst)
	asmInst.Kind = OtherInstruction

	switch inst.Op {
	case x86asm.CALL, x86asm.LCALL:
		asmInst.Kind = CallInstruction
	case x86asm.RET, x86asm.LRET:
		asmInst.Kind = RetInstruction
	}

	asmInst.DestLoc = resolveCallArgAMD64(&inst, asmInst.Loc.PC, asmInst.AtPC, regs, memrw, bi)
	return nil
}

func (i *I386) Prologues() []opcodeSeq {
	return prologuesAMD64
}

type i386ArchInst x86asm.Inst

func (inst *i386ArchInst) Text(flavour AssemblyFlavour, pc uint64, symLookup func(uint64) (string, uint64)) string {
	if inst == nil {
		return "?"
	}

	var text string

	switch flavour {
	case GNUFlavour:
		text = x86asm.GNUSyntax(x86asm.Inst(*inst), pc, symLookup)
	case GoFlavour:
		text = x86asm.GoSyntax(x86asm.Inst(*inst), pc, symLookup)
	case IntelFlavour:
		fallthrough
	default:
		text = x86asm.IntelSyntax(x86asm.Inst(*inst), pc, symLookup)
	}

	return text

}

func (inst *i386ArchInst) OpcodeEquals(op uint64) bool {
	if inst == nil {
		return false
	}
	return uint64(inst.Op) == op
}
