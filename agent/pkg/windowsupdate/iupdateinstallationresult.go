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

// IUpdateInstallationResult represents the result of an installation or uninstallation for an update.
// https://learn.microsoft.com/en-us/windows/win32/api/wuapi/nn-wuapi-iupdateinstallationresult
type IUpdateInstallationResult struct {
	disp           *ole.IDispatch
	HResult        int32
	RebootRequired bool
	ResultCode     int32 // OperationResultCode enum
}

func toIUpdateInstallationResult(disp *ole.IDispatch) (*IUpdateInstallationResult, error) {
	if disp == nil {
		return nil, nil
	}

	var err error
	r := &IUpdateInstallationResult{disp: disp}

	if r.HResult, err = toInt32Err(oleutil.GetProperty(disp, "HResult")); err != nil {
		return nil, err
	}

	if r.RebootRequired, err = toBoolErr(oleutil.GetProperty(disp, "RebootRequired")); err != nil {
		return nil, err
	}

	if r.ResultCode, err = toInt32Err(oleutil.GetProperty(disp, "ResultCode")); err != nil {
		return nil, err
	}

	return r, nil
}
