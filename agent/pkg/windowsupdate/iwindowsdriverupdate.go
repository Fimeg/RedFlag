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
	"time"

	"github.com/go-ole/go-ole"
	"github.com/go-ole/go-ole/oleutil"
)

// IWindowsDriverUpdate contains the properties and methods available to an update that installs a Windows driver.
// https://learn.microsoft.com/en-us/windows/win32/api/wuapi/nn-wuapi-iwindowsdriverupdate
type IWindowsDriverUpdate struct {
	*IUpdate
	DeviceProblemNumber int32
	DeviceStatus        int32
	DriverClass         string
	DriverHardwareID    string
	DriverManufacturer  string
	DriverModel         string
	DriverProvider      string
	DriverVerDate       *time.Time
	// IWindowsDriverUpdate2 properties
	RebootRequired2 bool
	IsPresent2      bool
	// IWindowsDriverUpdate3 properties
	BrowseOnly2 bool
	// IWindowsDriverUpdate4 properties
	WindowsDriverUpdateEntries []*IWindowsDriverUpdateEntry
	// IWindowsDriverUpdate5 properties
	AutoDownload2  int32
	AutoSelection2 int32
}

// ToWindowsDriverUpdate attempts to cast an IUpdate to IWindowsDriverUpdate.
// Returns nil if the update is not a driver update.
func (u *IUpdate) ToWindowsDriverUpdate() (*IWindowsDriverUpdate, error) {
	wdu := &IWindowsDriverUpdate{IUpdate: u}

	var err error

	// Try to get driver-specific properties
	if wdu.DeviceProblemNumber, err = toInt32Err(oleutil.GetProperty(u.disp, "DeviceProblemNumber")); err != nil {
		return nil, nil // Not a driver update
	}

	if wdu.DeviceStatus, err = toInt32Err(oleutil.GetProperty(u.disp, "DeviceStatus")); err != nil {
		return nil, err
	}

	if wdu.DriverClass, err = toStringErr(oleutil.GetProperty(u.disp, "DriverClass")); err != nil {
		return nil, err
	}

	if wdu.DriverHardwareID, err = toStringErr(oleutil.GetProperty(u.disp, "DriverHardwareID")); err != nil {
		return nil, err
	}

	if wdu.DriverManufacturer, err = toStringErr(oleutil.GetProperty(u.disp, "DriverManufacturer")); err != nil {
		return nil, err
	}

	if wdu.DriverModel, err = toStringErr(oleutil.GetProperty(u.disp, "DriverModel")); err != nil {
		return nil, err
	}

	if wdu.DriverProvider, err = toStringErr(oleutil.GetProperty(u.disp, "DriverProvider")); err != nil {
		return nil, err
	}

	if wdu.DriverVerDate, err = toTimeErr(oleutil.GetProperty(u.disp, "DriverVerDate")); err != nil {
		return nil, err
	}

	// IWindowsDriverUpdate4 - WindowsDriverUpdateEntries
	entriesDisp, err := toIDispatchErr(oleutil.GetProperty(u.disp, "WindowsDriverUpdateEntries"))
	if err == nil && entriesDisp != nil {
		wdu.WindowsDriverUpdateEntries, _ = toIWindowsDriverUpdateEntries(entriesDisp)
	}

	return wdu, nil
}

// IWindowsDriverUpdateEntry contains the properties that are available to a Windows driver update entry.
// https://learn.microsoft.com/en-us/windows/win32/api/wuapi/nn-wuapi-iwindowsdriverupdateentry
type IWindowsDriverUpdateEntry struct {
	disp                *ole.IDispatch
	DeviceProblemNumber int32
	DeviceStatus        int32
	DriverClass         string
	DriverHardwareID    string
	DriverManufacturer  string
	DriverModel         string
	DriverProvider      string
	DriverVerDate       *time.Time
}

func toIWindowsDriverUpdateEntry(disp *ole.IDispatch) (*IWindowsDriverUpdateEntry, error) {
	if disp == nil {
		return nil, nil
	}

	var err error
	entry := &IWindowsDriverUpdateEntry{disp: disp}

	if entry.DeviceProblemNumber, err = toInt32Err(oleutil.GetProperty(disp, "DeviceProblemNumber")); err != nil {
		return nil, err
	}

	if entry.DeviceStatus, err = toInt32Err(oleutil.GetProperty(disp, "DeviceStatus")); err != nil {
		return nil, err
	}

	if entry.DriverClass, err = toStringErr(oleutil.GetProperty(disp, "DriverClass")); err != nil {
		return nil, err
	}

	if entry.DriverHardwareID, err = toStringErr(oleutil.GetProperty(disp, "DriverHardwareID")); err != nil {
		return nil, err
	}

	if entry.DriverManufacturer, err = toStringErr(oleutil.GetProperty(disp, "DriverManufacturer")); err != nil {
		return nil, err
	}

	if entry.DriverModel, err = toStringErr(oleutil.GetProperty(disp, "DriverModel")); err != nil {
		return nil, err
	}

	if entry.DriverProvider, err = toStringErr(oleutil.GetProperty(disp, "DriverProvider")); err != nil {
		return nil, err
	}

	if entry.DriverVerDate, err = toTimeErr(oleutil.GetProperty(disp, "DriverVerDate")); err != nil {
		return nil, err
	}

	return entry, nil
}

func toIWindowsDriverUpdateEntries(disp *ole.IDispatch) ([]*IWindowsDriverUpdateEntry, error) {
	if disp == nil {
		return nil, nil
	}

	count, err := toInt32Err(oleutil.GetProperty(disp, "Count"))
	if err != nil {
		return nil, err
	}

	entries := make([]*IWindowsDriverUpdateEntry, 0, count)
	for i := int32(0); i < count; i++ {
		entryDisp, err := toIDispatchErr(oleutil.GetProperty(disp, "Item", i))
		if err != nil {
			return nil, err
		}

		entry, err := toIWindowsDriverUpdateEntry(entryDisp)
		if err != nil {
			return nil, err
		}

		entries = append(entries, entry)
	}
	return entries, nil
}
