package proc

import (
	"errors"
	"fmt"
	"go/ast"
	"go/constant"
	"reflect"
)

// Breakpoint represents a breakpoint. Stores information on the break
// point including the byte of data that originally was stored at that
// address.
type Breakpoint struct {
	// File & line information for printing.
	FunctionName string
	File         string
	Line         int

	Addr         uint64 // Address breakpoint is set for.
	OriginalData []byte // If software breakpoint, the data we replace with breakpoint instruction.
	Name         string // User defined name of the breakpoint
	ID           int    // Monotonically increasing ID.
	Temp         bool   // Whether this is a temp breakpoint (for next'ing).

	// Breakpoint information
	Tracepoint    bool     // Tracepoint flag
	Goroutine     bool     // Retrieve goroutine information
	Stacktrace    int      // Number of stack frames to retrieve
	Variables     []string // Variables to evaluate
	LoadArgs      *LoadConfig
	LoadLocals    *LoadConfig
	HitCount      map[int]uint64 // Number of times a breakpoint has been reached in a certain goroutine
	TotalHitCount uint64         // Number of times a breakpoint has been reached

	Cond ast.Expr // When Cond is not nil the breakpoint will be triggered only if evaluating Cond returns true
}

func (bp *Breakpoint) String() string {
	return fmt.Sprintf("Breakpoint %d at %#v %s:%d (%d)", bp.ID, bp.Addr, bp.File, bp.Line, bp.TotalHitCount)
}

// Clear this breakpoint appropriately depending on whether it is a
// hardware or software breakpoint.
func (bp *Breakpoint) Clear(thread *Thread) (*Breakpoint, error) {
	if _, err := thread.writeMemory(uintptr(bp.Addr), bp.OriginalData); err != nil {
		return nil, fmt.Errorf("could not clear breakpoint %s", err)
	}
	return bp, nil
}

// BreakpointExistsError is returned when trying to set a breakpoint at
// an address that already has a breakpoint set for it.
type BreakpointExistsError struct {
	file string
	line int
	addr uint64
}

func (bpe BreakpointExistsError) Error() string {
	return fmt.Sprintf("Breakpoint exists at %s:%d at %x", bpe.file, bpe.line, bpe.addr)
}

// InvalidAddressError represents the result of
// attempting to set a breakpoint at an invalid address.
type InvalidAddressError struct {
	address uint64
}

func (iae InvalidAddressError) Error() string {
	return fmt.Sprintf("Invalid address %#v\n", iae.address)
}

func (dbp *Process) setBreakpoint(tid int, addr uint64, temp bool) (*Breakpoint, error) {
	if bp, ok := dbp.FindBreakpoint(addr); ok {
		return nil, BreakpointExistsError{bp.File, bp.Line, bp.Addr}
	}

	f, l, fn := dbp.goSymTable.PCToLine(uint64(addr))
	if fn == nil {
		return nil, InvalidAddressError{address: addr}
	}

	newBreakpoint := &Breakpoint{
		FunctionName: fn.Name,
		File:         f,
		Line:         l,
		Addr:         addr,
		Temp:         temp,
		Cond:         nil,
		HitCount:     map[int]uint64{},
	}

	if temp {
		dbp.tempBreakpointIDCounter++
		newBreakpoint.ID = dbp.tempBreakpointIDCounter
	} else {
		dbp.breakpointIDCounter++
		newBreakpoint.ID = dbp.breakpointIDCounter
	}

	thread := dbp.Threads[tid]
	originalData, err := thread.readMemory(uintptr(addr), dbp.arch.BreakpointSize())
	if err != nil {
		return nil, err
	}
	if err := dbp.writeSoftwareBreakpoint(thread, addr); err != nil {
		return nil, err
	}
	newBreakpoint.OriginalData = originalData
	dbp.Breakpoints[addr] = newBreakpoint

	return newBreakpoint, nil
}

func (dbp *Process) writeSoftwareBreakpoint(thread *Thread, addr uint64) error {
	_, err := thread.writeMemory(uintptr(addr), dbp.arch.BreakpointInstruction())
	return err
}

func (bp *Breakpoint) checkCondition(thread *Thread) (bool, error) {
	if bp.Cond == nil {
		return true, nil
	}
	scope, err := thread.Scope()
	if err != nil {
		return true, err
	}
	v, err := scope.evalAST(bp.Cond)
	if err != nil {
		return true, fmt.Errorf("error evaluating expression: %v", err)
	}
	if v.Unreadable != nil {
		return true, fmt.Errorf("condition expression unreadable: %v", v.Unreadable)
	}
	if v.Kind != reflect.Bool {
		return true, errors.New("condition expression not boolean")
	}
	return constant.BoolVal(v.Value), nil
}

// NoBreakpointError is returned when trying to
// clear a breakpoint that does not exist.
type NoBreakpointError struct {
	addr uint64
}

func (nbp NoBreakpointError) Error() string {
	return fmt.Sprintf("no breakpoint at %#v", nbp.addr)
}
