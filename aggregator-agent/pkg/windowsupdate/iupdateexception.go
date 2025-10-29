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

// IUpdateException represents info about the aspects of search results returned in the ISearchResult object that were incomplete. For more info, see Remarks.
// https://docs.microsoft.com/en-us/windows/win32/api/wuapi/nn-wuapi-iupdateexception
type IUpdateException struct {
	disp    *ole.IDispatch
	Context int32 // enum https://docs.microsoft.com/en-us/windows/win32/api/wuapi/ne-wuapi-updateexceptioncontext
	HResult int64
	Message string
}

func toIUpdateExceptions(updateExceptionsDisp *ole.IDispatch) ([]*IUpdateException, error) {
	// TODO
	return nil, nil
}
