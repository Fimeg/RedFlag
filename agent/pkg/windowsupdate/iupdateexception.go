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

// IUpdateException represents info about the aspects of search results returned in the ISearchResult object that were incomplete. For more info, see Remarks.
// https://docs.microsoft.com/en-us/windows/win32/api/wuapi/nn-wuapi-iupdateexception
type IUpdateException struct {
	disp    *ole.IDispatch
	Context int32 // enum https://docs.microsoft.com/en-us/windows/win32/api/wuapi/ne-wuapi-updateexceptioncontext
	HResult int64
	Message string
}

func toIUpdateExceptions(updateExceptionsDisp *ole.IDispatch) ([]*IUpdateException, error) {
	count, err := toInt32Err(oleutil.GetProperty(updateExceptionsDisp, "Count"))
	if err != nil {
		return nil, err
	}

	exceptions := make([]*IUpdateException, 0, count)
	for i := 0; i < int(count); i++ {
		exceptionDisp, err := toIDispatchErr(oleutil.GetProperty(updateExceptionsDisp, "Item", i))
		if err != nil {
			return nil, err
		}

		exception, err := toIUpdateException(exceptionDisp)
		if err != nil {
			return nil, err
		}

		exceptions = append(exceptions, exception)
	}
	return exceptions, nil
}

func toIUpdateException(updateExceptionDisp *ole.IDispatch) (*IUpdateException, error) {
	var err error
	iUpdateException := &IUpdateException{
		disp: updateExceptionDisp,
	}

	if iUpdateException.Context, err = toInt32Err(oleutil.GetProperty(updateExceptionDisp, "Context")); err != nil {
		return nil, err
	}

	if iUpdateException.HResult, err = toInt64Err(oleutil.GetProperty(updateExceptionDisp, "HResult")); err != nil {
		return nil, err
	}

	if iUpdateException.Message, err = toStringErr(oleutil.GetProperty(updateExceptionDisp, "Message")); err != nil {
		return nil, err
	}

	return iUpdateException, nil
}
