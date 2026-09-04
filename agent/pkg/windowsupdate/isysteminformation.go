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

// ISystemInformation contains information about the specified computer.
// https://learn.microsoft.com/en-us/windows/win32/api/wuapi/nn-wuapi-isysteminformation
type ISystemInformation struct {
	disp                   *ole.IDispatch
	OemHardwareSupportLink string
	RebootRequired         bool
}

// NewSystemInformation creates a new ISystemInformation instance.
func NewSystemInformation() (*ISystemInformation, error) {
	unknown, err := oleutil.CreateObject("Microsoft.Update.SystemInfo")
	if err != nil {
		return nil, err
	}

	disp, err := unknown.QueryInterface(ole.IID_IDispatch)
	if err != nil {
		return nil, err
	}

	return toISystemInformation(disp)
}

func toISystemInformation(systemInfoDisp *ole.IDispatch) (*ISystemInformation, error) {
	var err error
	iSystemInformation := &ISystemInformation{
		disp: systemInfoDisp,
	}

	if iSystemInformation.OemHardwareSupportLink, err = toStringErr(oleutil.GetProperty(systemInfoDisp, "OemHardwareSupportLink")); err != nil {
		return nil, err
	}

	if iSystemInformation.RebootRequired, err = toBoolErr(oleutil.GetProperty(systemInfoDisp, "RebootRequired")); err != nil {
		return nil, err
	}

	return iSystemInformation, nil
}
