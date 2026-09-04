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

// IDownloadProgress represents the progress of an asynchronous download operation.
// https://learn.microsoft.com/en-us/windows/win32/api/wuapi/nn-wuapi-idownloadprogress
type IDownloadProgress struct {
	disp                         *ole.IDispatch
	CurrentUpdateBytesDownloaded int64
	CurrentUpdateBytesToDownload int64
	CurrentUpdateDownloadPhase   int32 // DownloadPhase enum
	CurrentUpdateIndex           int32
	CurrentUpdatePercentComplete int32
	PercentComplete              int32
	TotalBytesDownloaded         int64
	TotalBytesToDownload         int64
}

func toIDownloadProgress(disp *ole.IDispatch) (*IDownloadProgress, error) {
	if disp == nil {
		return nil, nil
	}

	var err error
	p := &IDownloadProgress{disp: disp}

	if p.CurrentUpdateBytesDownloaded, err = toInt64Err(oleutil.GetProperty(disp, "CurrentUpdateBytesDownloaded")); err != nil {
		return nil, err
	}

	if p.CurrentUpdateBytesToDownload, err = toInt64Err(oleutil.GetProperty(disp, "CurrentUpdateBytesToDownload")); err != nil {
		return nil, err
	}

	if p.CurrentUpdateDownloadPhase, err = toInt32Err(oleutil.GetProperty(disp, "CurrentUpdateDownloadPhase")); err != nil {
		return nil, err
	}

	if p.CurrentUpdateIndex, err = toInt32Err(oleutil.GetProperty(disp, "CurrentUpdateIndex")); err != nil {
		return nil, err
	}

	if p.CurrentUpdatePercentComplete, err = toInt32Err(oleutil.GetProperty(disp, "CurrentUpdatePercentComplete")); err != nil {
		return nil, err
	}

	if p.PercentComplete, err = toInt32Err(oleutil.GetProperty(disp, "PercentComplete")); err != nil {
		return nil, err
	}

	if p.TotalBytesDownloaded, err = toInt64Err(oleutil.GetProperty(disp, "TotalBytesDownloaded")); err != nil {
		return nil, err
	}

	if p.TotalBytesToDownload, err = toInt64Err(oleutil.GetProperty(disp, "TotalBytesToDownload")); err != nil {
		return nil, err
	}

	return p, nil
}

// GetUpdateResult returns the result of the download for a specified update.
// https://learn.microsoft.com/en-us/windows/win32/api/wuapi/nf-wuapi-idownloadprogress-getupdateresult
func (p *IDownloadProgress) GetUpdateResult(updateIndex int32) (*IUpdateDownloadResult, error) {
	resultDisp, err := toIDispatchErr(oleutil.CallMethod(p.disp, "GetUpdateResult", updateIndex))
	if err != nil {
		return nil, err
	}
	return toIUpdateDownloadResult(resultDisp)
}
