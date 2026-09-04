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

// IUpdateDownloader downloads updates from the server.
// https://docs.microsoft.com/en-us/windows/win32/api/wuapi/nn-wuapi-iupdatedownloaders
type IUpdateDownloader struct {
	disp                *ole.IDispatch
	ClientApplicationID string
	IsForced            bool
	Priority            int32 // enum https://docs.microsoft.com/en-us/windows/win32/api/wuapi/ne-wuapi-downloadpriority
	Updates             []*IUpdate
}

func toIUpdateDownloader(updateDownloaderDisp *ole.IDispatch) (*IUpdateDownloader, error) {
	var err error
	iUpdateDownloader := &IUpdateDownloader{
		disp: updateDownloaderDisp,
	}

	if iUpdateDownloader.ClientApplicationID, err = toStringErr(oleutil.GetProperty(updateDownloaderDisp, "ClientApplicationID")); err != nil {
		return nil, err
	}

	if iUpdateDownloader.IsForced, err = toBoolErr(oleutil.GetProperty(updateDownloaderDisp, "IsForced")); err != nil {
		return nil, err
	}

	if iUpdateDownloader.Priority, err = toInt32Err(oleutil.GetProperty(updateDownloaderDisp, "Priority")); err != nil {
		return nil, err
	}

	updatesDisp, err := toIDispatchErr(oleutil.GetProperty(updateDownloaderDisp, "Updates"))
	if err != nil {
		return nil, err
	}
	if updatesDisp != nil {
		if iUpdateDownloader.Updates, err = toIUpdates(updatesDisp); err != nil {
			return nil, err
		}
	}

	return iUpdateDownloader, nil
}

// Download starts a synchronous download of the content files that are associated with the updates.
// https://docs.microsoft.com/en-us/windows/win32/api/wuapi/nf-wuapi-iupdatedownloader-download
func (iUpdateDownloader *IUpdateDownloader) Download(updates []*IUpdate) (*IDownloadResult, error) {
	updatesDisp, err := toIUpdateCollection(updates)
	if err != nil {
		return nil, err
	}
	if _, err = oleutil.PutProperty(iUpdateDownloader.disp, "Updates", updatesDisp); err != nil {
		return nil, err
	}

	downloadResultDisp, err := toIDispatchErr(oleutil.CallMethod(iUpdateDownloader.disp, "Download"))
	if err != nil {
		return nil, err
	}
	return toIDownloadResult(downloadResultDisp)
}

// BeginDownload begins an asynchronous download of the content files associated with the updates.
// https://learn.microsoft.com/en-us/windows/win32/api/wuapi/nf-wuapi-iupdatedownloader-begindownload
func (iUpdateDownloader *IUpdateDownloader) BeginDownload(updates []*IUpdate) (*IDownloadJob, error) {
	updatesDisp, err := toIUpdateCollection(updates)
	if err != nil {
		return nil, err
	}
	if _, err = oleutil.PutProperty(iUpdateDownloader.disp, "Updates", updatesDisp); err != nil {
		return nil, err
	}

	jobDisp, err := toIDispatchErr(oleutil.CallMethod(iUpdateDownloader.disp, "BeginDownload", nil, nil, nil))
	if err != nil {
		return nil, err
	}
	return toIDownloadJob(jobDisp)
}

// EndDownload completes an asynchronous download.
// https://learn.microsoft.com/en-us/windows/win32/api/wuapi/nf-wuapi-iupdatedownloader-enddownload
func (iUpdateDownloader *IUpdateDownloader) EndDownload(downloadJob *IDownloadJob) (*IDownloadResult, error) {
	resultDisp, err := toIDispatchErr(oleutil.CallMethod(iUpdateDownloader.disp, "EndDownload", downloadJob.disp))
	if err != nil {
		return nil, err
	}
	return toIDownloadResult(resultDisp)
}

// PutClientApplicationID sets the identifier of the current client application.
func (iUpdateDownloader *IUpdateDownloader) PutClientApplicationID(value string) error {
	_, err := oleutil.PutProperty(iUpdateDownloader.disp, "ClientApplicationID", value)
	if err != nil {
		return err
	}
	iUpdateDownloader.ClientApplicationID = value
	return nil
}

// PutIsForced sets whether the download is forced.
func (iUpdateDownloader *IUpdateDownloader) PutIsForced(value bool) error {
	_, err := oleutil.PutProperty(iUpdateDownloader.disp, "IsForced", value)
	if err != nil {
		return err
	}
	iUpdateDownloader.IsForced = value
	return nil
}

// PutPriority sets the download priority.
func (iUpdateDownloader *IUpdateDownloader) PutPriority(value int32) error {
	_, err := oleutil.PutProperty(iUpdateDownloader.disp, "Priority", value)
	if err != nil {
		return err
	}
	iUpdateDownloader.Priority = value
	return nil
}
