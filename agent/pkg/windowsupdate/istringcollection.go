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

// IStringCollection represents a collection of strings.
// https://learn.microsoft.com/en-us/windows/win32/api/wuapi/nn-wuapi-istringcollection
type IStringCollection struct {
	disp     *ole.IDispatch
	Count    int32
	ReadOnly bool
}

// NewStringCollection creates a new empty IStringCollection.
func NewStringCollection() (*IStringCollection, error) {
	unknown, err := oleutil.CreateObject("Microsoft.Update.StringColl")
	if err != nil {
		return nil, err
	}

	disp, err := unknown.QueryInterface(ole.IID_IDispatch)
	if err != nil {
		return nil, err
	}

	return toIStringCollection(disp)
}

func toIStringCollection(disp *ole.IDispatch) (*IStringCollection, error) {
	if disp == nil {
		return nil, nil
	}

	var err error
	sc := &IStringCollection{disp: disp}

	if sc.Count, err = toInt32Err(oleutil.GetProperty(disp, "Count")); err != nil {
		return nil, err
	}

	if sc.ReadOnly, err = toBoolErr(oleutil.GetProperty(disp, "ReadOnly")); err != nil {
		return nil, err
	}

	return sc, nil
}

// Item gets a string from the collection at the specified index.
func (sc *IStringCollection) Item(index int32) (string, error) {
	return toStringErr(oleutil.GetProperty(sc.disp, "Item", index))
}

// Add adds a string to the collection.
func (sc *IStringCollection) Add(value string) (int32, error) {
	result, err := oleutil.CallMethod(sc.disp, "Add", value)
	if err != nil {
		return 0, err
	}
	sc.Count++
	return variantToInt32(result), nil
}

// Clear removes all items from the collection.
func (sc *IStringCollection) Clear() error {
	_, err := oleutil.CallMethod(sc.disp, "Clear")
	if err == nil {
		sc.Count = 0
	}
	return err
}

// Insert inserts a string at the specified position.
func (sc *IStringCollection) Insert(index int32, value string) error {
	_, err := oleutil.CallMethod(sc.disp, "Insert", index, value)
	if err == nil {
		sc.Count++
	}
	return err
}

// RemoveAt removes the item at the specified index.
func (sc *IStringCollection) RemoveAt(index int32) error {
	_, err := oleutil.CallMethod(sc.disp, "RemoveAt", index)
	if err == nil {
		sc.Count--
	}
	return err
}

// ToSlice converts the collection to a Go string slice.
func (sc *IStringCollection) ToSlice() ([]string, error) {
	result := make([]string, sc.Count)
	for i := int32(0); i < sc.Count; i++ {
		str, err := sc.Item(i)
		if err != nil {
			return nil, err
		}
		result[i] = str
	}
	return result, nil
}

// iStringCollectionToStringArrayErr is a helper function for backward compatibility.
func iStringCollectionToStringArrayErr(disp *ole.IDispatch, err error) ([]string, error) {
	if err != nil {
		return nil, err
	}

	if disp == nil {
		return nil, nil
	}

	count, err := toInt32Err(oleutil.GetProperty(disp, "Count"))
	if err != nil {
		return nil, err
	}

	stringCollection := make([]string, count)

	for i := 0; i < int(count); i++ {
		str, err := toStringErr(oleutil.GetProperty(disp, "Item", i))
		if err != nil {
			return nil, err
		}

		stringCollection[i] = str
	}
	return stringCollection, nil
}
