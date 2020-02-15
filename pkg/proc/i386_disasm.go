// TODO: disassembler support should be compiled in unconditionally,
// instead of being decided by the build-target architecture, and be
// part of the Arch object instead.

package proc

import (
	"encoding/binary"
	"golang.org/x/arch/x86/x86asm"
)

// AsmDecode decodes the assembly instruction starting at mem[0:] into asmInst.
// It assumes that the Loc and AtPC fields of asmInst have already been filled.
func (i *I386) AsmDecode(asmInst *AsmInstruction, mem []byte, regs Registers, memrw MemoryReadWriter, bi *BinaryInfo) error {
	inst, err := x86asm.Decode(mem, 32)
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

	asmInst.DestLoc = resolveCallArgI386(&inst, asmInst.Loc.PC, asmInst.AtPC, regs, memrw, bi)
	return nil
}

func (i *I386) Prologues() []opcodeSeq {
	return prologuesI386
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

func resolveCallArgI386(inst *x86asm.Inst, instAddr uint64, currentGoroutine bool, regs Registers, mem MemoryReadWriter, bininfo *BinaryInfo) *Location {
	if inst.Op != x86asm.CALL && inst.Op != x86asm.LCALL {
		return nil
	}

	var pc uint64
	var err error

	switch arg := inst.Args[0].(type) {
	case x86asm.Imm:
		pc = uint64(arg)
	case x86asm.Reg:
		if !currentGoroutine || regs == nil {
			return nil
		}
		pc, err = regs.Get(int(arg))
		if err != nil {
			return nil
		}
	case x86asm.Mem:
		if !currentGoroutine || regs == nil {
			return nil
		}
		if arg.Segment != 0 {
			return nil
		}
		base, err1 := regs.Get(int(arg.Base))
		index, err2 := regs.Get(int(arg.Index))
		if err1 != nil || err2 != nil {
			return nil
		}
		addr := uintptr(int64(base) + int64(index*uint64(arg.Scale)) + arg.Disp)
		//TODO: should this always be 32 bits instead of inst.MemBytes?
		pcbytes := make([]byte, inst.MemBytes)
		_, err := mem.ReadMemory(pcbytes, addr)
		if err != nil {
			return nil
		}
		pc = uint64(binary.LittleEndian.Uint32(pcbytes))
	default:
		return nil
	}

	file, line, fn := bininfo.PCToLine(pc)
	if fn == nil {
		return &Location{PC: pc}
	}
	return &Location{PC: pc, File: file, Line: line, Fn: fn}
}

// Possible stacksplit prologues are inserted by stacksplit in
// $GOROOT/src/cmd/internal/obj/x86/obj6.go.
// The stacksplit prologue will always begin with loading curg in CX, this
// instruction is added by load_g_cx in the same file and is either 1 or 2
// MOVs.
var prologuesI386 []opcodeSeq

func init() {
	var tinyStacksplit = opcodeSeq{uint64(x86asm.CMP), uint64(x86asm.JBE)}
	var smallStacksplit = opcodeSeq{uint64(x86asm.LEA), uint64(x86asm.CMP), uint64(x86asm.JBE)}
	var bigStacksplit = opcodeSeq{uint64(x86asm.MOV), uint64(x86asm.CMP), uint64(x86asm.JE), uint64(x86asm.LEA), uint64(x86asm.SUB), uint64(x86asm.CMP), uint64(x86asm.JBE)}
	var unixGetG = opcodeSeq{uint64(x86asm.MOV)}
	var windowsGetG = opcodeSeq{uint64(x86asm.MOV), uint64(x86asm.MOV)}

	prologuesI386 = make([]opcodeSeq, 0, 2*3)
	for _, getG := range []opcodeSeq{unixGetG, windowsGetG} {
		for _, stacksplit := range []opcodeSeq{tinyStacksplit, smallStacksplit, bigStacksplit} {
			prologue := make(opcodeSeq, 0, len(getG)+len(stacksplit))
			prologue = append(prologue, getG...)
			prologue = append(prologue, stacksplit...)
			prologuesI386 = append(prologuesI386, prologue)
		}
	}
}
