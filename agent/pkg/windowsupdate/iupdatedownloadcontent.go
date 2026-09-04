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

// IUpdateDownloadContent represents the download content of an update.
// https://docs.microsoft.com/en-us/windows/win32/api/wuapi/nn-wuapi-iupdatedownloadcontent
type IUpdateDownloadContent struct {
	disp        *ole.IDispatch
	DownloadUrl string
}

func toIUpdateDownloadContents(updateDownloadContentsDisp *ole.IDispatch) ([]*IUpdateDownloadContent, error) {
	count, err := toInt32Err(oleutil.GetProperty(updateDownloadContentsDisp, "Count"))
	if err != nil {
		return nil, err
	}

	contents := make([]*IUpdateDownloadContent, 0, count)
	for i := 0; i < int(count); i++ {
		contentDisp, err := toIDispatchErr(oleutil.GetProperty(updateDownloadContentsDisp, "Item", i))
		if err != nil {
			return nil, err
		}

		content, err := toIUpdateDownloadContent(contentDisp)
		if err != nil {
			return nil, err
		}

		contents = append(contents, content)
	}
	return contents, nil
}

func toIUpdateDownloadContent(updateDownloadContentDisp *ole.IDispatch) (*IUpdateDownloadContent, error) {
	var err error
	iUpdateDownloadContent := &IUpdateDownloadContent{
		disp: updateDownloadContentDisp,
	}

	if iUpdateDownloadContent.DownloadUrl, err = toStringErr(oleutil.GetProperty(updateDownloadContentDisp, "DownloadUrl")); err != nil {
		return nil, err
	}

	return iUpdateDownloadContent, nil
}
