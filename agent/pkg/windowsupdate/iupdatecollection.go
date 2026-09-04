/*
Copyright 2022 Zheng Dayu
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
    http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package windowsupdate

import (
	"github.com/go-ole/go-ole"
	"github.com/go-ole/go-ole/oleutil"
)

// IUpdateCollection represents a collection of updates.
// https://learn.microsoft.com/en-us/windows/win32/api/wuapi/nn-wuapi-iupdatecollection
type IUpdateCollection struct {
	disp     *ole.IDispatch
	Count    int32
	ReadOnly bool
}

// NewUpdateCollection creates a new empty IUpdateCollection.
func NewUpdateCollection() (*IUpdateCollection, error) {
	unknown, err := oleutil.CreateObject("Microsoft.Update.UpdateColl")
	if err != nil {
		return nil, err
	}

	disp, err := unknown.QueryInterface(ole.IID_IDispatch)
	if err != nil {
		return nil, err
	}

	return toIUpdateCollection2(disp)
}

func toIUpdateCollection2(disp *ole.IDispatch) (*IUpdateCollection, error) {
	if disp == nil {
		return nil, nil
	}

	var err error
	uc := &IUpdateCollection{disp: disp}

	if uc.Count, err = toInt32Err(oleutil.GetProperty(disp, "Count")); err != nil {
		return nil, err
	}

	if uc.ReadOnly, err = toBoolErr(oleutil.GetProperty(disp, "ReadOnly")); err != nil {
		return nil, err
	}

	return uc, nil
}

// Item gets an update from the collection at the specified index.
func (uc *IUpdateCollection) Item(index int32) (*IUpdate, error) {
	itemDisp, err := toIDispatchErr(oleutil.GetProperty(uc.disp, "Item", index))
	if err != nil {
		return nil, err
	}
	return toIUpdate(itemDisp)
}

// Add adds an update to the collection.
func (uc *IUpdateCollection) Add(update *IUpdate) (int32, error) {
	result, err := oleutil.CallMethod(uc.disp, "Add", update.disp)
	if err != nil {
		return 0, err
	}
	uc.Count++
	return variantToInt32(result), nil
}

// Clear removes all items from the collection.
func (uc *IUpdateCollection) Clear() error {
	_, err := oleutil.CallMethod(uc.disp, "Clear")
	if err == nil {
		uc.Count = 0
	}
	return err
}

// Copy creates a shallow copy of the collection.
func (uc *IUpdateCollection) Copy() (*IUpdateCollection, error) {
	copyDisp, err := toIDispatchErr(oleutil.CallMethod(uc.disp, "Copy"))
	if err != nil {
		return nil, err
	}
	return toIUpdateCollection2(copyDisp)
}

// Insert inserts an update at the specified position.
func (uc *IUpdateCollection) Insert(index int32, update *IUpdate) error {
	_, err := oleutil.CallMethod(uc.disp, "Insert", index, update.disp)
	if err == nil {
		uc.Count++
	}
	return err
}

// RemoveAt removes the item at the specified index.
func (uc *IUpdateCollection) RemoveAt(index int32) error {
	_, err := oleutil.CallMethod(uc.disp, "RemoveAt", index)
	if err == nil {
		uc.Count--
	}
	return err
}

// ToSlice converts the collection to a Go slice.
func (uc *IUpdateCollection) ToSlice() ([]*IUpdate, error) {
	result := make([]*IUpdate, uc.Count)
	for i := int32(0); i < uc.Count; i++ {
		update, err := uc.Item(i)
		if err != nil {
			return nil, err
		}
		result[i] = update
	}
	return result, nil
}

// GetDispatch returns the underlying IDispatch for use with other WUA methods.
func (uc *IUpdateCollection) GetDispatch() *ole.IDispatch {
	return uc.disp
}
