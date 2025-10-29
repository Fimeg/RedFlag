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
)

// IUpdateDownloadContent represents the download content of an update.
// https://docs.microsoft.com/en-us/windows/win32/api/wuapi/nn-wuapi-iupdatedownloadcontent
type IUpdateDownloadContent struct {
	disp        *ole.IDispatch
	DownloadUrl string
}

func toIUpdateDownloadContents(updateDownloadContentsDisp *ole.IDispatch) ([]*IUpdateDownloadContent, error) {
	// TODO
	return nil, nil
}
