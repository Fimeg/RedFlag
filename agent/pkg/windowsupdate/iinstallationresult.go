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

// IInstallationResult represents the result of an installation or uninstallation.
// https://docs.microsoft.com/en-us/windows/win32/api/wuapi/nn-wuapi-iinstallationresult
type IInstallationResult struct {
	disp           *ole.IDispatch
	HResult        int32
	RebootRequired bool
	ResultCode     int32 // enum https://docs.microsoft.com/en-us/windows/win32/api/wuapi/ne-wuapi-operationresultcode
}

func toIInstallationResult(installationResultDisp *ole.IDispatch) (*IInstallationResult, error) {
	var err error
	iInstallationResult := &IInstallationResult{
		disp: installationResultDisp,
	}

	if iInstallationResult.HResult, err = toInt32Err(oleutil.GetProperty(installationResultDisp, "HResult")); err != nil {
		return nil, err
	}

	if iInstallationResult.RebootRequired, err = toBoolErr(oleutil.GetProperty(installationResultDisp, "RebootRequired")); err != nil {
		return nil, err
	}

	if iInstallationResult.ResultCode, err = toInt32Err(oleutil.GetProperty(installationResultDisp, "ResultCode")); err != nil {
		return nil, err
	}

	return iInstallationResult, nil
}

// GetUpdateResult returns an IInstallationResult interface that contains the installation information for a specified update.
// https://learn.microsoft.com/en-us/windows/win32/api/wuapi/nn-wuapi-iinstallationresult
func (iInstallationResult *IInstallationResult) GetUpdateResult(updateIndex int32) (*IInstallationResult, error) {
	var err error
	iUpdateInstallationResult := &IInstallationResult{
		disp: iInstallationResult.disp,
	}
	updatesDisp, err := toIDispatchErr(oleutil.CallMethod(iInstallationResult.disp, "GetUpdateResult", updateIndex))
	if err != nil {
		return nil, err
	}

	if iUpdateInstallationResult.HResult, err = toInt32Err(oleutil.GetProperty(updatesDisp, "HResult")); err != nil {
		return nil, err
	}

	if iUpdateInstallationResult.ResultCode, err = toInt32Err(oleutil.GetProperty(updatesDisp, "ResultCode")); err != nil {
		return nil, err
	}
	return iUpdateInstallationResult, nil
}
