package main

import (
	"fmt"

	"github.com/onflow/flow-go/fvm/state"
	"github.com/onflow/flow-go/model/flow"
)

type RegisterGetRegisterFunc func(string, string) (flow.RegisterValue, error)

type RemoteView struct {
	Parent *RemoteView
	Delta  map[string]flow.RegisterValue

	getRemoteRegister RegisterGetRegisterFunc
}

func NewRemoteView(getRemoteRegister RegisterGetRegisterFunc) *RemoteView {

	view := &RemoteView{
		Delta:             make(map[string]flow.RegisterValue),
		getRemoteRegister: getRemoteRegister,
	}
	return view
}

func (v *RemoteView) NewChild() state.View {
	return &RemoteView{
		Parent: v,
		Delta:  make(map[string][]byte),
	}
}

func (v *RemoteView) MergeView(o state.View) error {
	var other *RemoteView
	var ok bool
	if other, ok = o.(*RemoteView); !ok {
		return fmt.Errorf("can not merge: view type mismatch (given: %T, expected:RemoteView)", o)
	}

	for k, value := range other.Delta {
		v.Delta[k] = value
	}
	return nil
}

func (v *RemoteView) DropDelta() {
	v.Delta = make(map[string]flow.RegisterValue)
}

func (v *RemoteView) Set(owner, key string, value flow.RegisterValue) error {
	v.Delta[owner+"~"+key] = value
	return nil
}

func (v *RemoteView) Get(owner, key string) (flow.RegisterValue, error) {

	value, found := v.Delta[owner+"~"+key]
	if found {
		return value, nil
	}

	if v.Parent != nil {
		return v.Parent.Get(owner, key)
	}

	resp, err := v.getRemoteRegister(owner, key)
	if err != nil {
		return nil, err
	}

	return resp, nil
}

func (v *RemoteView) AllRegisters() []flow.RegisterID {
	panic("Not implemented")
}

func (v *RemoteView) RegisterUpdates() ([]flow.RegisterID, []flow.RegisterValue) {
	panic("Not implemented")
}

func (v *RemoteView) Touch(owner, key string) error {
	return nil
}

func (v *RemoteView) Delete(owner, key string) error {
	v.Delta[owner+"~"+key] = nil
	return nil
}
