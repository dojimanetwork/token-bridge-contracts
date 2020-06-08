/*
* Copyright 2020, Offchain Labs, Inc.
*
* Licensed under the Apache License, Version 2.0 (the "License");
* you may not use this file except in compliance with the License.
* You may obtain a copy of the License at
*
*    http://www.apache.org/licenses/LICENSE-2.0
*
* Unless required by applicable law or agreed to in writing, software
* distributed under the License is distributed on an "AS IS" BASIS,
* WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
* See the License for the specific language governing permissions and
* limitations under the License.
 */

package message

import (
	"bytes"
	"errors"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common/hexutil"

	"github.com/offchainlabs/arbitrum/packages/arb-util/common"
	"github.com/offchainlabs/arbitrum/packages/arb-util/hashing"
	"github.com/offchainlabs/arbitrum/packages/arb-util/value"
)

type Call struct {
	To   common.Address
	From common.Address
	Data []byte
}

func (m Call) String() string {
	return fmt.Sprintf("Transaction(to: %v, from: %v, data: %v)",
		m.To,
		m.From,
		m.Data,
	)
}

func (m Call) GetFuncName() string {
	return hexutil.Encode(m.Data[:4])
}

func (m Call) Equals(other Message) bool {
	o, ok := other.(Call)
	if !ok {
		return false
	}
	return m.To == o.To &&
		m.From == o.From &&
		bytes.Equal(m.Data, o.Data)
}

func (m Call) Type() Type {
	return CallType
}

func (m Call) CommitmentHash() common.Hash {
	return hashing.SoliditySHA3(
		hashing.Uint8(uint8(m.Type())),
		hashing.Address(m.To),
		hashing.Address(m.From),
		hashing.Bytes32(hashing.SoliditySHA3(m.Data)),
	)
}

func (m Call) AsInboxValue() value.TupleValue {
	val1, _ := value.NewTupleFromSlice([]value.Value{
		addressToIntValue(m.To),
		BytesToByteStack(m.Data),
	})
	val2, _ := value.NewTupleFromSlice([]value.Value{
		value.NewIntValue(big.NewInt(int64(m.Type()))),
		addressToIntValue(m.From),
		val1,
	})
	return val2
}

func UnmarshalCall(val value.Value) (Call, error) {
	from, tup, err := unmarshalTxWrapped(val, CallType)
	failRet := Call{}
	if err != nil {
		return failRet, err
	}

	if tup.Len() != 2 {
		return failRet, fmt.Errorf("expected tuple of length 2, but recieved %v", tup)
	}
	destVal, _ := tup.GetByInt64(0)
	dataVal, _ := tup.GetByInt64(1)

	destInt, ok := destVal.(value.IntValue)
	if !ok {
		return failRet, errors.New("dest must be an int")
	}
	data, err := ByteStackToHex(dataVal)
	if err != nil {
		return failRet, err
	}

	return Call{
		To:   intValueToAddress(destInt),
		From: from,
		Data: data,
	}, nil
}

func (m Call) ReceiptHash() common.Hash {
	return hashing.SoliditySHA3(
		hashing.Uint8(uint8(m.Type())),
		hashing.Address(m.To),
		hashing.Address(m.From),
		m.Data,
	)
}

func (m Call) CheckpointValue() value.Value {
	val, _ := value.NewTupleFromSlice([]value.Value{
		addressToIntValue(m.To),
		addressToIntValue(m.From),
		BytesToByteStack(m.Data),
	})
	return val
}

func UnmarshalCallFromCheckpoint(v value.Value) (Call, error) {
	tup, ok := v.(value.TupleValue)
	failRet := Call{}
	if !ok || tup.Len() != 3 {
		return failRet, errors.New("tx val must be 3-tuple")
	}
	to, _ := tup.GetByInt64(0)
	toInt, ok := to.(value.IntValue)
	if !ok {
		return failRet, errors.New("to must be int")
	}
	from, _ := tup.GetByInt64(1)
	fromInt, ok := from.(value.IntValue)
	if !ok {
		return failRet, errors.New("from must be int")
	}
	data, _ := tup.GetByInt64(2)
	dataBytes, err := ByteStackToHex(data)
	if err != nil {
		return failRet, err
	}

	return Call{
		To:   intValueToAddress(toInt),
		From: intValueToAddress(fromInt),
		Data: dataBytes,
	}, nil
}
