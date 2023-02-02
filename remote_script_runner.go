package main

import (
	"github.com/rs/zerolog"
	"math"

	"github.com/onflow/cadence"
	"github.com/onflow/flow-go/fvm"
	"github.com/onflow/flow-go/fvm/state"
	"github.com/onflow/flow-go/model/flow"
)

type RemoteDebugger struct {
	vm   *fvm.VirtualMachine
	ctx  fvm.Context
	view state.View
}

func NewRemoteDebugger(
	view *RemoteView,
	chain flow.Chain,
	logger zerolog.Logger) *RemoteDebugger {
	vm := fvm.NewVirtualMachine()

	ctx := fvm.NewContext(
		fvm.WithLogger(logger),
		fvm.WithChain(chain),
		fvm.WithComputationLimit(math.MaxUint64),
		fvm.WithMemoryLimit(math.MaxUint64),
		fvm.WithMaxStateInteractionSize(math.MaxUint64),
		fvm.WithContractRemovalRestricted(false),
		fvm.WithAuthorizationChecksEnabled(false),
		fvm.WithSequenceNumberCheckAndIncrementEnabled(false),
	)

	return &RemoteDebugger{
		ctx:  ctx,
		vm:   vm,
		view: view,
	}
}

func (d *RemoteDebugger) RunScript(script *fvm.ScriptProcedure) (value cadence.Value, scriptError, processError error) {
	scriptCtx := fvm.NewContextFromParent(d.ctx)
	err := d.vm.Run(scriptCtx, script, d.view)
	if err != nil {
		return nil, nil, err
	}
	return script.Value, script.Err, nil
}

func (d *RemoteDebugger) RunTransaction(tx *fvm.TransactionProcedure) (txError, processError error) {
	scriptCtx := fvm.NewContextFromParent(d.ctx)
	err := d.vm.Run(scriptCtx, tx, d.view)
	if err != nil {
		return nil, err
	}
	return tx.Err, nil
}
