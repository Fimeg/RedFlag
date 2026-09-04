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

// IDownloadResult represents the result of a download operation.
// https://docs.microsoft.com/en-us/windows/win32/api/wuapi/nn-wuapi-idownloadresult
type IDownloadResult struct {
	disp       *ole.IDispatch
	HResult    int32
	ResultCode int32 // enum https://docs.microsoft.com/en-us/windows/win32/api/wuapi/ne-wuapi-operationresultcode
}

func toIDownloadResult(downloadResultDisp *ole.IDispatch) (*IDownloadResult, error) {
	var err error
	iDownloadResult := &IDownloadResult{
		disp: downloadResultDisp,
	}

	if iDownloadResult.HResult, err = toInt32Err(oleutil.GetProperty(downloadResultDisp, "HResult")); err != nil {
		return nil, err
	}

	if iDownloadResult.ResultCode, err = toInt32Err(oleutil.GetProperty(downloadResultDisp, "ResultCode")); err != nil {
		return nil, err
	}

	return iDownloadResult, nil
}

// GetUpdateResult returns an IUpdateDownloadResult interface that contains the download information for a specified update.
// https://docs.microsoft.com/en-us/windows/win32/api/wuapi/nf-wuapi-idownloadresult-getupdateresult
func (iDownloadResult *IDownloadResult) GetUpdateResult(updateIndex int32) (*IUpdateDownloadResult, error) {
	var err error
	iUpdateDownloadResult := &IUpdateDownloadResult{
		disp: iDownloadResult.disp,
	}
	updatesDisp, err := toIDispatchErr(oleutil.CallMethod(iDownloadResult.disp, "GetUpdateResult", updateIndex))
	if err != nil {
		return nil, err
	}

	if iUpdateDownloadResult.HResult, err = toInt32Err(oleutil.GetProperty(updatesDisp, "HResult")); err != nil {
		return nil, err
	}

	if iUpdateDownloadResult.ResultCode, err = toInt32Err(oleutil.GetProperty(updatesDisp, "ResultCode")); err != nil {
		return nil, err
	}
	return iUpdateDownloadResult, nil
}
