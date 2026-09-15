// Copyright 2017 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package tracers

import (
	"bytes"
	"encoding/json"
	"errors"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/MetisProtocol/mvm/l2geth/common"
	"github.com/MetisProtocol/mvm/l2geth/core/state"
	"github.com/MetisProtocol/mvm/l2geth/core/vm"
	"github.com/MetisProtocol/mvm/l2geth/params"
)

type account struct{}

func (account) SubBalance(amount *big.Int)                          {}
func (account) AddBalance(amount *big.Int)                          {}
func (account) SetAddress(common.Address)                           {}
func (account) Value() *big.Int                                     { return nil }
func (account) SetBalance(*big.Int)                                 {}
func (account) SetNonce(uint64)                                     {}
func (account) Balance() *big.Int                                   { return nil }
func (account) Address() common.Address                             { return common.Address{} }
func (account) ReturnGas(*big.Int)                                  {}
func (account) SetCode(common.Hash, []byte)                         {}
func (account) ForEachStorage(cb func(key, value common.Hash) bool) {}

type dummyStatedb struct {
	state.StateDB
}

func (*dummyStatedb) GetRefund() uint64 { return 1337 }

func runTrace(tracer *Tracer) (json.RawMessage, error) {
	env := vm.NewEVM(vm.Context{BlockNumber: big.NewInt(1)}, &dummyStatedb{}, params.TestChainConfig, vm.Config{Debug: true, Tracer: tracer})

	contract := vm.NewContract(account{}, account{}, big.NewInt(0), 10000)
	contract.Code = []byte{byte(vm.PUSH1), 0x1, byte(vm.PUSH1), 0x1, 0x0}

	_, err := env.Interpreter().Run(contract, []byte{}, false)
	if err != nil {
		return nil, err
	}
	return tracer.GetResult()
}

func TestTracing(t *testing.T) {
	tracer, err := New("{count: 0, step: function() { this.count += 1; }, fault: function() {}, result: function() { return this.count; }}")
	if err != nil {
		t.Fatal(err)
	}

	ret, err := runTrace(tracer)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(ret, []byte("3")) {
		t.Errorf("Expected return value to be 3, got %s", string(ret))
	}
}

func TestStack(t *testing.T) {
	tracer, err := New("{depths: [], step: function(log) { this.depths.push(log.stack.length()); }, fault: function() {}, result: function() { return this.depths; }}")
	if err != nil {
		t.Fatal(err)
	}

	ret, err := runTrace(tracer)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(ret, []byte("[0,1,2]")) {
		t.Errorf("Expected return value to be [0,1,2], got %s", string(ret))
	}
}

func TestOpcodes(t *testing.T) {
	tracer, err := New("{opcodes: [], step: function(log) { this.opcodes.push(log.op.toString()); }, fault: function() {}, result: function() { return this.opcodes; }}")
	if err != nil {
		t.Fatal(err)
	}

	ret, err := runTrace(tracer)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(ret, []byte("[\"PUSH1\",\"PUSH1\",\"STOP\"]")) {
		t.Errorf("Expected return value to be [\"PUSH1\",\"PUSH1\",\"STOP\"], got %s", string(ret))
	}
}

func TestHalt(t *testing.T) {
	timeout := errors.New("stahp")
	tracer, err := New("{step: function() { while(1); }, fault: function() {}, result: function() { return null; }}")
	if err != nil {
		t.Fatal(err)
	}

	go func() {
		time.Sleep(1 * time.Second)
		tracer.Stop(timeout)
	}()

	if _, err = runTrace(tracer); err == nil || err.Error() != "stahp    in server-side tracer function 'step'" {
		t.Errorf("Expected timeout error, got %v", err)
	}
}

func TestHaltBetweenSteps(t *testing.T) {
	tracer, err := New("{step: function() {}, fault: function() {}, result: function() { return null; }}")
	if err != nil {
		t.Fatal(err)
	}

	env := vm.NewEVM(vm.Context{BlockNumber: big.NewInt(1)}, &dummyStatedb{}, params.TestChainConfig, vm.Config{Debug: true, Tracer: tracer})
	contract := vm.NewContract(&account{}, &account{}, big.NewInt(0), 0)

	tracer.CaptureState(env, 0, 0, 0, 0, nil, nil, contract, 0, nil)
	timeout := errors.New("stahp")
	tracer.Stop(timeout)
	tracer.CaptureState(env, 0, 0, 0, 0, nil, nil, contract, 0, nil)

	if _, err := tracer.GetResult(); err == nil || err.Error() != timeout.Error() {
		t.Errorf("Expected timeout error, got %v", err)
	}
}

func TestGojaBuiltins(t *testing.T) {
	tests := []struct{ expression, want string }{
		{`toHex(toWord("0x1234"))`, `"0x0000000000000000000000000000000000000000000000000000000000001234"`},
		{`toHex(toAddress("0x1234"))`, `"0x0000000000000000000000000000000000001234"`},
		{`toHex(slice(new Uint8Array([0, 128, 255]), 1, 3))`, `"0x80ff"`},
		{`toHex(new Uint8Array([1, 2, 3]).subarray(1))`, `"0x0203"`},
		{`toHex(slice(toWord("0x01"), -1, 2))`, `"0x"`},
		{`isPrecompiled(toAddress("0x01"))`, `true`},
		{`isPrecompiled(toAddress("0xff"))`, `false`},
		{`toHex(toContract("0x0000000000000000000000000000000000000000", 0))`, `"0xbd770416a3345f91e4b34576cb804a576fa48eb1"`},
		{`toHex(toContract2(toAddress("0x00"), toWord("0x00"), new Uint8Array([0])))`, `"0x4d1a2e2bb4f88f0250f26ffff098b0b30b26bf38"`},
		{`bigInt("115792089237316195423570985008687907853269984665640564039457584007913129639935").toString(16)`, `"ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"`},
		{`({toJSON: function() { return "custom"; }})`, `"custom"`},
		{`undefined`, `null`},
	}
	for _, test := range tests {
		t.Run(test.expression, func(t *testing.T) {
			tracer, err := New(`{step: function(){}, fault: function(){}, result: function(){return ` + test.expression + `;}}`)
			if err != nil {
				t.Fatal(err)
			}
			result, err := tracer.GetResult()
			if err != nil {
				t.Fatal(err)
			}
			if string(result) != test.want {
				t.Fatalf("got %s, want %s", result, test.want)
			}
		})
	}
}

func TestGojaValidation(t *testing.T) {
	for _, code := range []string{
		`null`, `42`, `({})`, `{step: 1, fault: function(){}, result: function(){}}`,
		`{get step(){throw new Error("getter");}, fault: function(){}, result: function(){}}`,
		`{step: function(){}, fault: null, result: function(){}}`,
		`{step: function(){}, fault: function(){}, result: "bad"}`, `syntax error`,
	} {
		if _, err := New(code); err == nil {
			t.Errorf("accepted invalid tracer %s", code)
		}
	}
	for name := range all {
		if _, err := New(name); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestGojaResultErrors(t *testing.T) {
	for _, expression := range []string{
		`toHex(null)`, `toHex({})`, `(function(){throw new Error("result failure")})()`,
		`(function(){var a={}; a.self=a; return a;})()`,
		`({toJSON: function(){throw new Error("JSON failure");}})`,
	} {
		tracer, err := New(`{step: function(){}, fault: function(){}, result: function(){return ` + expression + `;}}`)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := tracer.GetResult(); err == nil {
			t.Errorf("expected error for %s", expression)
		}
	}
}

func TestGojaBuffersAndContext(t *testing.T) {
	tracer, err := New(`{
  saved: null,
  step: function(log) {
   if (this.saved === null) {
    this.saved = log.contract.getInput();
    this.saved[0] = 255;
   }
   if (toHex(log.contract.getInput()) !== "0x0102") throw new Error("aliased input");
   if (toHex(log.memory.slice(-1, 0)) !== "0x") throw new Error("negative memory slice");
   if (toHex(log.memory.slice(2, 1)) !== "0x") throw new Error("reversed memory slice");
   if (log.memory.getUint(-1).toString() !== "0") throw new Error("negative memory offset");
   if (log.stack.peek(-1).toString() !== "0") throw new Error("negative stack index");
   if (log.getRefund() !== 1337) throw new Error("refund");
  },
  fault: function(){},
  result: function(ctx){return [toHex(this.saved), toHex(ctx.input), toHex(ctx.output), ctx.value.toString(16), ctx.type, ctx.gas, ctx.gasUsed];}
 }`)
	if err != nil {
		t.Fatal(err)
	}
	input := []byte{1, 2}
	value := new(big.Int).Lsh(big.NewInt(1), 255)
	tracer.CaptureStart(common.Address{}, common.Address{}, false, input, 10000, value)
	env := vm.NewEVM(vm.Context{BlockNumber: big.NewInt(1)}, &dummyStatedb{}, params.TestChainConfig, vm.Config{Debug: true, Tracer: tracer})
	contract := vm.NewContract(account{}, account{}, value, 10000)
	contract.Code = []byte{byte(vm.PUSH1), 1, byte(vm.STOP)}
	if _, err := env.Interpreter().Run(contract, input, false); err != nil {
		t.Fatal(err)
	}
	tracer.CaptureEnd([]byte{3}, 3, time.Second, nil)
	result, err := tracer.GetResult()
	if err != nil {
		t.Fatal(err)
	}
	want := `["0xff02","0x0102","0x03","8000000000000000000000000000000000000000000000000000000000000000","CALL",10000,3]`
	if string(result) != want {
		t.Fatalf("got %s, want %s", result, want)
	}
	if !bytes.Equal(input, []byte{1, 2}) {
		t.Fatal("input mutated")
	}
}

func TestGojaHaltResult(t *testing.T) {
	tracer, err := New(`{step: function(){}, fault: function(){}, result: function(){while(true){}}}`)
	if err != nil {
		t.Fatal(err)
	}
	timer := time.AfterFunc(50*time.Millisecond, func() { tracer.Stop(errors.New("result timeout")) })
	defer timer.Stop()
	if _, err := tracer.GetResult(); err == nil {
		t.Fatal("expected interruption")
	}
}

func TestGojaCallbackErrors(t *testing.T) {
	for _, method := range []string{"step", "fault"} {
		t.Run(method, func(t *testing.T) {
			tracer, err := New(`{step: function(){}, fault: function(){}, result: function(){return null;}, ` + method + `: function(){throw new Error("callback failure");}}`)
			if err != nil {
				t.Fatal(err)
			}
			if method == "step" {
				_, err = runTrace(tracer)
			} else {
				tracer.CaptureFault(nil, 0, 0, 0, 0, nil, nil, nil, 0, errors.New("evm failure"))
				_, err = tracer.GetResult()
			}
			if err == nil || !strings.Contains(err.Error(), "callback failure") || !strings.Contains(err.Error(), "'"+method+"'") {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestGojaConcurrentStop(t *testing.T) {
	for i := 0; i < 30; i++ {
		tracer, err := New(`{step: function(){}, fault: function(){}, result: function(){while(true){}}}`)
		if err != nil {
			t.Fatal(err)
		}
		tracer.CaptureStart(common.Address{}, common.Address{}, false, []byte{1}, 100, big.NewInt(1))
		done := make(chan struct{})
		go func() { defer close(done); tracer.Stop(errors.New("stopped")); tracer.Stop(errors.New("again")) }()
		if _, err := tracer.GetResult(); err == nil {
			t.Fatal("expected interruption")
		}
		<-done
	}
}
