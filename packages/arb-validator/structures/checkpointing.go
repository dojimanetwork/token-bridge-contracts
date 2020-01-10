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

package structures

import (
	"context"
	"github.com/offchainlabs/arbitrum/packages/arb-util/protocol"
	"log"
	"math/big"
	"os"
	"sync"

	"github.com/ethereum/go-ethereum/common"
	"github.com/gogo/protobuf/proto"
	"github.com/offchainlabs/arbitrum/packages/arb-avm-cpp/cmachine"
	"github.com/offchainlabs/arbitrum/packages/arb-util/machine"
	"github.com/offchainlabs/arbitrum/packages/arb-util/utils"
	"github.com/offchainlabs/arbitrum/packages/arb-util/value"
	"github.com/offchainlabs/arbitrum/packages/arb-validator/loader"
)

type CheckpointContext interface {
	AddValue(value.Value)
	AddMachine(machine.Machine)
	Manifest() *CheckpointManifest
	Values() map[[32]byte]value.Value
	Machines() map[[32]byte]machine.Machine
}

type CheckpointContextImpl struct {
	values   map[[32]byte]value.Value
	machines map[[32]byte]machine.Machine
}

type RestoreContext interface {
	GetValue([32]byte) value.Value
	GetMachine([32]byte) machine.Machine
}

func NewCheckpointContextImpl() *CheckpointContextImpl {
	return &CheckpointContextImpl{
		values:   make(map[[32]byte]value.Value),
		machines: make(map[[32]byte]machine.Machine),
	}
}

func (ctx *CheckpointContextImpl) AddValue(val value.Value) {
	ctx.values[val.Hash()] = val
}

func (ctx *CheckpointContextImpl) AddMachine(mach machine.Machine) {
	if ctx.machines[mach.Hash()] == nil {
		ctx.machines[mach.Hash()] = mach.Clone()
	}
}

func (ctx *CheckpointContextImpl) Manifest() *CheckpointManifest {
	vals := []*value.HashBuf{}
	for h, _ := range ctx.values {
		vals = append(vals, utils.MarshalHash(h))
	}
	machines := []*value.HashBuf{}
	for h, _ := range ctx.machines {
		machines = append(machines, utils.MarshalHash(h))
	}
	return &CheckpointManifest{Values: vals, Machines: machines}
}

func (ctx *CheckpointContextImpl) Values() map[[32]byte]value.Value {
	return ctx.values
}

func (ctx *CheckpointContextImpl) Machines() map[[32]byte]machine.Machine {
	return ctx.machines
}

func (ctx *CheckpointContextImpl) GetValue(h [32]byte) value.Value {
	return ctx.values[h]
}

func (ctx *CheckpointContextImpl) GetMachine(h [32]byte) machine.Machine {
	return ctx.machines[h]
}

type RollupCheckpointer struct {
	maxReorgDepth *big.Int
	cp            checkpointerWithMetadata
	asyncWriter   *AsyncCheckpointWriter
}

const checkpointDatabasePathBase = "/tmp/arb-validator-checkpoint-"

func makeCheckpointDatabasePath(rollupAddr common.Address) string {
	return checkpointDatabasePathBase + rollupAddr.Hex()[2:]
}

func NewRollupCheckpointer(
	ctx context.Context,
	rollupAddr common.Address,
	arbitrumCodeFilePath string,
	maxReorgDepth *big.Int,
) *RollupCheckpointer {
	return NewRollupCheckpointerWithType(ctx, rollupAddr, arbitrumCodeFilePath, maxReorgDepth, "")
}

func NewRollupCheckpointerWithType(
	ctx context.Context,
	rollupAddr common.Address,
	arbitrumCodeFilePath string,
	maxReorgDepth *big.Int,
	checkpointerType string,
) *RollupCheckpointer {
	databasePath := makeCheckpointDatabasePath(rollupAddr)
	switch checkpointerType {
	case "inmemory_testing": // inefficient in-memory checkpointer, for testing
		ret := &RollupCheckpointer{
			maxReorgDepth: maxReorgDepth,
			cp:            newDummyCheckpointer(arbitrumCodeFilePath),
		}
		ret.asyncWriter = NewAsyncCheckpointWriter(ctx, ret)
		return ret
	case "fresh_rocksdb": // for testing only -- use rocksdb but delete old database first
		if err := os.RemoveAll(databasePath); err != nil {
			log.Fatal(err)
		}
		fallthrough
	case "": // empty string gives you what you want for production
		fallthrough
	case "rocksdb": // store in rocksdb database, keyed to rollupAddr -- use this for production
		ret := &RollupCheckpointer{
			maxReorgDepth: maxReorgDepth,
			cp:            newProductionCheckpointer(databasePath, arbitrumCodeFilePath),
		}
		ret.asyncWriter = NewAsyncCheckpointWriter(ctx, ret)
		return ret
	default:
		return nil
	}
}

func (rcp *RollupCheckpointer) _saveCheckpoint(
	blockHeight *big.Int,
	contents []byte,
	checkpointCtx CheckpointContext,
) error {
	// read in metadata
	var metadataBuf *CheckpointMetadata
	var newestInCp *big.Int
	rawMetadata := rcp.cp.RestoreMetadata()

	// read in metadata, or create it if it doesn't already exist
	if rawMetadata == nil || len(rawMetadata) == 0 {
		heightBuf := utils.MarshalBigInt(blockHeight)
		metadataBuf = &CheckpointMetadata{
			FormatVersion:     1,
			OldestBlockHeight: heightBuf,
			NewestBlockHeight: heightBuf,
		}
		buf, err := proto.Marshal(metadataBuf)
		if err != nil {
			return err
		}
		rcp.cp.SaveMetadata(buf)
	} else {
		metadataBuf = &CheckpointMetadata{}
		if err := proto.Unmarshal(rawMetadata, metadataBuf); err != nil {
			return err
		}
	}
	newestInCp = utils.UnmarshalBigInt(metadataBuf.NewestBlockHeight)

	// save all of the data for this checkpoint
	rcp.cp.SaveCheckpoint(
		blockHeight,
		newestInCp,
		contents,
		checkpointCtx.Manifest(),
		checkpointCtx.Values(),
		checkpointCtx.Machines(),
	)

	// update the metadata to include this checkpoint
	metadataBuf.NewestBlockHeight = utils.MarshalBigInt(blockHeight)
	buf, err := proto.Marshal(metadataBuf)
	if err != nil {
		return err
	}
	rcp.cp.SaveMetadata(buf)

	return nil
}

func (rcp *RollupCheckpointer) RestoreCheckpoint(blockHeight *big.Int) ([]byte, RestoreContext, error) {
	var metadataBuf *CheckpointMetadata
	var oldestInCp *big.Int
	var newestInCp *big.Int
	rawMetadata := rcp.cp.RestoreMetadata()
	if rawMetadata == nil {
		return nil, nil, nil
	}

	metadataBuf = &CheckpointMetadata{}
	if err := proto.Unmarshal(rawMetadata, metadataBuf); err != nil {
		return nil, nil, err
	}
	oldestInCp = utils.UnmarshalBigInt(metadataBuf.OldestBlockHeight)
	newestInCp = utils.UnmarshalBigInt(metadataBuf.NewestBlockHeight)

	if blockHeight.Cmp(oldestInCp) < 0 || blockHeight.Cmp(newestInCp) > 0 {
		return nil, nil, nil
	}

	buf, checkpointCtx := rcp.cp.RestoreCheckpoint(blockHeight)
	return buf, checkpointCtx, nil
}

func (cp *RollupCheckpointer) GetInitialMachine() (machine.Machine, error) {
	return cp.cp.GetInitialMachine()
}

func (cp *RollupCheckpointer) AsyncSaveCheckpoint(
	blocknum *protocol.TimeBlocks,
	buf []byte,
	cpCtx CheckpointContext,
	doneChan chan interface{},
) {
	cp.asyncWriter.SubmitJob(
		func() {
			cp._saveCheckpoint(blocknum.AsInt(), buf, cpCtx)
		},
		doneChan,
	)
}

type AsyncCheckpointWriter struct {
	*sync.Mutex
	checkpointer *RollupCheckpointer
	notifyChan   chan interface{}
	nextJob      func()
	doneChans    []chan interface{}
}

func NewAsyncCheckpointWriter(ctx context.Context, cp *RollupCheckpointer) *AsyncCheckpointWriter {
	ret := &AsyncCheckpointWriter{&sync.Mutex{}, cp, make(chan interface{}, 1), nil, nil}
	go func() {
		for {
			select {
			case <-ret.notifyChan:
				ret.Lock()
				job := ret.nextJob
				if job != nil {
					ret.nextJob = nil
				}
				doneChansCopy := append([]chan interface{}{}, ret.doneChans...)
				ret.Unlock()
				if job != nil {
					job()
				}
				for _, dc := range doneChansCopy {
					if dc != nil {
						close(dc)
					}
				}
			case <-ctx.Done():
				return
			}
		}
	}()
	return ret
}

func (acw *AsyncCheckpointWriter) SubmitJob(job func(), doneChan chan interface{}) {
	acw.Lock()
	defer acw.Unlock()
	acw.nextJob = job
	acw.doneChans = append(acw.doneChans, doneChan)
	select {
	case acw.notifyChan <- nil:
		// do nothing; only purpose was to send on the channel
	default:
		// no need to do anything, because channel already has something in it
	}
}

type checkpointerWithMetadata interface {
	SaveMetadata([]byte)
	RestoreMetadata() []byte
	SaveCheckpoint(
		blockHeight *big.Int,
		prevHeight *big.Int,
		contents []byte,
		manifest *CheckpointManifest,
		values map[[32]byte]value.Value,
		machines map[[32]byte]machine.Machine,
	)
	RestoreCheckpoint(blockHeight *big.Int) ([]byte, RestoreContext) // returns nil, nil if no data at blockHeight
	DeleteCheckpoint(blockHeight *big.Int)

	GetInitialMachine() (machine.Machine, error)
}

type dummyCheckpointer struct {
	metadata       []byte
	cp             map[*big.Int]*dummyCheckpoint
	initialMachine machine.Machine
}

func newDummyCheckpointer(contractPath string) *dummyCheckpointer {
	theMachine, err := loader.LoadMachineFromFile(contractPath, true, "test")
	if err != nil {
		log.Fatal("newDummyCheckpointer: error loading ", contractPath)
	}
	return &dummyCheckpointer{
		nil,
		make(map[*big.Int]*dummyCheckpoint),
		theMachine,
	}
}

type dummyCheckpoint struct {
	contents []byte
	manifest *CheckpointManifest
	values   map[[32]byte]value.Value
	machines map[[32]byte]machine.Machine
}

func (dcp *dummyCheckpoint) GetValue(h [32]byte) value.Value {
	return dcp.values[h]
}

func (dcp *dummyCheckpoint) GetMachine(h [32]byte) machine.Machine {
	return dcp.machines[h]
}

func (cp *dummyCheckpointer) SaveMetadata(data []byte) {
	cp.metadata = append([]byte{}, data...)
}

func (cp *dummyCheckpointer) RestoreMetadata() []byte {
	return append([]byte{}, cp.metadata...)
}

func (cp *dummyCheckpointer) SaveCheckpoint(
	blockHeight *big.Int,
	prevBlockHeight *big.Int,
	contents []byte,
	manifest *CheckpointManifest,
	values map[[32]byte]value.Value,
	machines map[[32]byte]machine.Machine,
) {
	cp.cp[blockHeight] = &dummyCheckpoint{contents, manifest, values, machines}
}

func (cp *dummyCheckpointer) RestoreCheckpoint(blockHeight *big.Int) ([]byte, RestoreContext) {
	dcp := cp.cp[blockHeight]
	if dcp == nil {
		return nil, nil
	} else {
		return dcp.contents, dcp
	}
}

func (cp *dummyCheckpointer) DeleteCheckpoint(blockHeight *big.Int) {
	delete(cp.cp, blockHeight)
}

func (cp *dummyCheckpointer) GetInitialMachine() (machine.Machine, error) {
	return cp.initialMachine.Clone(), nil
}

var metadataKey []byte
var contentsKey []byte

func init() {
	metadataKey = []byte("metadata")
	contentsKey = []byte("contents")
}

func manifestKey(blockHeight *big.Int) []byte {
	bhBytes := blockHeight.Bytes()
	return append([]byte("manifest:"), bhBytes...)
}

func linksKey(blockHeight *big.Int) []byte {
	bhBytes := blockHeight.Bytes()
	return append([]byte("links:"), bhBytes...)
}

func lastTouchedKey(h [32]byte, isMachine bool) []byte {
	if isMachine {
		return append([]byte{73, 0}, h[:]...)
	} else {
		return append([]byte{73, 1}, h[:]...)
	}
}

type productionCheckpointer struct {
	st machine.CheckpointStorage
}

func newProductionCheckpointer(dbpath, contractpath string) *productionCheckpointer {
	checkpoint, err := cmachine.NewCheckpoint(dbpath, contractpath)
	if err != nil {
		log.Fatal(err)
	}
	return &productionCheckpointer{checkpoint}
}

func (csc *productionCheckpointer) SaveMetadata(data []byte) {
	ok := csc.st.SaveData(metadataKey, data)
	if !ok {
		log.Fatal("metadata checkpointing failure")
	}
}

func (csc *productionCheckpointer) RestoreMetadata() []byte {
	return csc.st.GetData(metadataKey)
}

func (csc *productionCheckpointer) SaveCheckpoint(
	blockHeight *big.Int,
	prevBlockHeight *big.Int,
	contents []byte,
	manifest *CheckpointManifest,
	values map[[32]byte]value.Value,
	machines map[[32]byte]machine.Machine,
) {
	blockHeightBuf := utils.MarshalBigInt(blockHeight)
	blockHeightBytes, err := proto.Marshal(blockHeightBuf)
	if err != nil {
		log.Fatal(err)
	}

	for h, val := range values {
		csc.st.SaveData(lastTouchedKey(h, false), blockHeightBytes)
		csc.st.SaveValue(val)
	}

	for h, mach := range machines {
		csc.st.SaveData(lastTouchedKey(h, false), blockHeightBytes)
		mach.Checkpoint(csc.st)
	}

	manifestBuf, err := proto.Marshal(manifest)
	if err != nil {
		log.Fatal(err)
	}
	csc.st.SaveData(manifestKey(blockHeight), manifestBuf)

	csc.st.SaveData(contentsKey, contents)

	csc._updateNextPointer(prevBlockHeight, blockHeight)
	csc._setBothPointers(blockHeight, prevBlockHeight, blockHeight)
}

func (csc *productionCheckpointer) _setBothPointers(idx, prev, next *big.Int) {
	links := &CheckpointLinks{
		PrevBlockHeight: utils.MarshalBigInt(prev),
		NextBlockHeight: utils.MarshalBigInt(next),
	}
	linksBuf, err := proto.Marshal(links)
	if err != nil {
		log.Fatal(err)
	}
	csc.st.SaveData(linksKey(idx), linksBuf)
}

func (csc *productionCheckpointer) _updatePrevPointer(idx, prev *big.Int) {
	key := linksKey(idx)
	linksBuf := csc.st.GetData(key)
	links := &CheckpointLinks{}
	if err := proto.Unmarshal(linksBuf, links); err != nil {
		log.Fatal(err)
	}
	links.PrevBlockHeight = utils.MarshalBigInt(prev)
	linksBuf, err := proto.Marshal(links)
	if err != nil {
		log.Fatal(err)
	}
	csc.st.SaveData(key, linksBuf)
}

func (csc *productionCheckpointer) _updateNextPointer(idx, next *big.Int) {
	key := linksKey(idx)
	linksBuf := csc.st.GetData(key)
	links := &CheckpointLinks{}
	if err := proto.Unmarshal(linksBuf, links); err != nil {
		log.Fatal(err)
	}
	links.NextBlockHeight = utils.MarshalBigInt(next)
	linksBuf, err := proto.Marshal(links)
	if err != nil {
		log.Fatal(err)
	}
	csc.st.SaveData(key, linksBuf)
}

func (csc *productionCheckpointer) RestoreCheckpoint(blockHeight *big.Int) ([]byte, RestoreContext) { // returns nil, nil if no data at blockHeight
	// check for consistency with metadata
	metadataBytes := csc.RestoreMetadata()
	metadataBuf := &CheckpointMetadata{}
	if err := proto.Unmarshal(metadataBytes, metadataBuf); err != nil {
		log.Fatal(err)
	}
	oldestHeight := utils.UnmarshalBigInt(metadataBuf.OldestBlockHeight)
	newestHeight := utils.UnmarshalBigInt(metadataBuf.NewestBlockHeight)
	if blockHeight.Cmp(oldestHeight) < 0 || blockHeight.Cmp(newestHeight) > 0 {
		return nil, nil
	}

	// read contents
	contentBytes := csc.st.GetData(contentsKey)

	return contentBytes, csc
}

func (csc *productionCheckpointer) DeleteCheckpoint(blockHeight *big.Int) {
	// make a best effort to delete an old checkpoint, but ignore any errors
	// errors might cause some harmless extra info to remain in the database
	//
	// this assumes it's being called on the oldest remaining checkpoint
	// if that's not true, older checkpoints will remain harmlessly in the database

	// update metadata to reflect deletion
	metadataBytes := csc.RestoreMetadata()
	metadataBuf := &CheckpointMetadata{}
	if err := proto.Unmarshal(metadataBytes, metadataBuf); err != nil {
		return
	}
	oldestHeight := utils.UnmarshalBigInt(metadataBuf.OldestBlockHeight)
	newestHeight := utils.UnmarshalBigInt(metadataBuf.NewestBlockHeight)
	if blockHeight.Cmp(newestHeight) >= 0 {
		// deleted the last item, so null the metadata
		csc.SaveMetadata([]byte{})
	} else if blockHeight.Cmp(oldestHeight) > 0 {
		metadataBuf.OldestBlockHeight = utils.MarshalBigInt(blockHeight)
		var err error
		metadataBytes, err = proto.Marshal(metadataBuf)
		if err != nil {
			return
		}
		csc.SaveMetadata(metadataBytes)
	}

	manifestBytes := csc.st.GetData(manifestKey(blockHeight))
	if manifestBytes == nil {
		return
	}
	manifestBuf := &CheckpointManifest{}
	if err := proto.Unmarshal(manifestBytes, manifestBuf); err != nil {
		return
	}
	csc.st.DeleteData(manifestKey(blockHeight))
	for _, vbuf := range manifestBuf.Values {
		valhash := utils.UnmarshalHash(vbuf)
		csc.st.DeleteValue(valhash)
	}
	for _, mbuf := range manifestBuf.Machines {
		machhash := utils.UnmarshalHash(mbuf)
		csc.st.DeleteCheckpoint(machhash)
	}
	csc.st.DeleteData(contentsKey)
}

func (csc *productionCheckpointer) GetValue(h [32]byte) value.Value {
	return csc.st.GetValue(h)
}

func (csc *productionCheckpointer) GetMachine(h [32]byte) machine.Machine {
	ret, err := csc.st.GetInitialMachine()
	if err != nil {
		log.Fatal(err)
	}
	ret.RestoreCheckpoint(csc.st, h)
	return ret
}

func (csc *productionCheckpointer) GetInitialMachine() (machine.Machine, error) {
	return csc.st.GetInitialMachine()
}
