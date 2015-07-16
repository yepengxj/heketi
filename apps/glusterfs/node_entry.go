//
// Copyright (c) 2015 The heketi Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

package glusterfs

import (
	"bytes"
	"encoding/gob"
	"errors"
	"github.com/boltdb/bolt"
	"github.com/heketi/heketi/utils"
	"github.com/lpabon/godbc"
	"sort"
)

type NodeEntry struct {
	Info    NodeInfo
	Devices sort.StringSlice
}

func NewNodeEntry() *NodeEntry {
	entry := &NodeEntry{}
	entry.Devices = make(sort.StringSlice, 0)

	return entry
}

func NewNodeEntryFromRequest(req *NodeAddRequest) *NodeEntry {
	godbc.Require(req != nil)

	node := NewNodeEntry()
	node.Info.Id = utils.GenUUID()
	node.Info.ClusterId = req.ClusterId
	node.Info.Hostnames = req.Hostnames
	node.Info.Zone = req.Zone

	return node
}

func NewNodeEntryFromId(tx *bolt.Tx, id string) (*NodeEntry, error) {
	godbc.Require(tx != nil)

	entry := NewNodeEntry()
	b := tx.Bucket([]byte(BOLTDB_BUCKET_NODE))
	if b == nil {
		logger.LogError("Unable to access node bucket")
		err := errors.New("Unable to create node entry")
		return nil, err
	}

	val := b.Get([]byte(id))
	if val == nil {
		return nil, ErrNotFound
	}

	err := entry.Unmarshal(val)
	if err != nil {
		logger.LogError("Unable to unmarshal node: %v", err)
		return nil, err
	}

	return entry, nil
}

func (n *NodeEntry) Save(tx *bolt.Tx) error {
	godbc.Require(tx != nil)
	godbc.Require(len(n.Info.Id) > 0)

	// Access bucket
	b := tx.Bucket([]byte(BOLTDB_BUCKET_NODE))
	if b == nil {
		err := errors.New("Unable to create node entry")
		logger.Err(err)
		return err
	}

	// Save node entry to db
	buffer, err := n.Marshal()
	if err != nil {
		logger.Err(err)
		return err
	}

	// Save data using the id as the key
	err = b.Put([]byte(n.Info.Id), buffer)
	if err != nil {
		logger.Err(err)
		return err
	}

	return nil

}

func (n *NodeEntry) Delete(tx *bolt.Tx) error {
	godbc.Require(tx != nil)

	// Check if the nodes still has drives
	if len(n.Devices) > 0 {
		logger.Warning("Unable to delete node [%v] because it contains devices", n.Info.Id)
		return ErrConflict
	}

	b := tx.Bucket([]byte(BOLTDB_BUCKET_NODE))
	if b == nil {
		err := errors.New("Unable to access database")
		logger.Err(err)
		return err
	}

	// Delete key
	err := b.Delete([]byte(n.Info.Id))
	if err != nil {
		logger.LogError("Unable to delete container key [%v] in db: %v", n.Info.Id, err.Error())
		return err
	}

	return nil
}

func (n *NodeEntry) NewInfoReponse(tx *bolt.Tx) (*NodeInfoResponse, error) {

	godbc.Require(tx != nil)

	info := &NodeInfoResponse{}
	info.ClusterId = n.Info.ClusterId
	info.Hostnames = n.Info.Hostnames
	info.Id = n.Info.Id
	info.Storage = n.Info.Storage
	info.Zone = n.Info.Zone
	info.DevicesInfo = make([]DeviceInfoResponse, 0)

	// Add each drive information
	for _, deviceid := range n.Devices {
		device, err := NewDeviceEntryFromId(tx, deviceid)
		if err != nil {
			return nil, err
		}

		driveinfo, err := device.NewInfoResponse(tx)
		if err != nil {
			return nil, err
		}
		info.DevicesInfo = append(info.DevicesInfo, *driveinfo)
	}

	return info, nil
}

func (n *NodeEntry) Marshal() ([]byte, error) {
	var buffer bytes.Buffer
	enc := gob.NewEncoder(&buffer)
	err := enc.Encode(*n)

	return buffer.Bytes(), err
}

func (n *NodeEntry) Unmarshal(buffer []byte) error {
	dec := gob.NewDecoder(bytes.NewReader(buffer))
	err := dec.Decode(n)
	if err != nil {
		return err
	}

	// Make sure to setup arrays if nil
	if n.Devices == nil {
		n.Devices = make(sort.StringSlice, 0)
	}

	return nil
}

func (n *NodeEntry) DeviceAdd(id string) {
	godbc.Require(!utils.SortedStringHas(n.Devices, id))

	n.Devices = append(n.Devices, id)
	n.Devices.Sort()
}

func (n *NodeEntry) DeviceDelete(id string) {
	n.Devices = utils.SortedStringsDelete(n.Devices, id)
}

func (n *NodeEntry) StorageAdd(amount uint64) {
	n.Info.Storage.Free += amount
	n.Info.Storage.Total += amount
}

func (n *NodeEntry) StorageAllocate(amount uint64) {
	n.Info.Storage.Free -= amount
	n.Info.Storage.Used += amount
}

func (n *NodeEntry) StorageFree(amount uint64) {
	n.Info.Storage.Free += amount
	n.Info.Storage.Used -= amount
}

func (n *NodeEntry) StorageDelete(amount uint64) {
	n.Info.Storage.Total -= amount
	n.Info.Storage.Free -= amount
}
