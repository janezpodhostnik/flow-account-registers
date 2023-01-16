package main

import (
	"github.com/rs/zerolog"

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
