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

// ISearchJob contains properties and methods that are available to a search operation.
// https://learn.microsoft.com/en-us/windows/win32/api/wuapi/nn-wuapi-isearchjob
type ISearchJob struct {
	disp        *ole.IDispatch
	AsyncState  interface{}
	IsCompleted bool
}

func toISearchJob(disp *ole.IDispatch) (*ISearchJob, error) {
	if disp == nil {
		return nil, nil
	}

	var err error
	j := &ISearchJob{disp: disp}

	if j.IsCompleted, err = toBoolErr(oleutil.GetProperty(disp, "IsCompleted")); err != nil {
		return nil, err
	}

	return j, nil
}

// CleanUp releases the resources held by the search job.
// https://learn.microsoft.com/en-us/windows/win32/api/wuapi/nf-wuapi-isearchjob-cleanup
func (j *ISearchJob) CleanUp() error {
	_, err := oleutil.CallMethod(j.disp, "CleanUp")
	return err
}

// RequestAbort requests that the search job be canceled.
// https://learn.microsoft.com/en-us/windows/win32/api/wuapi/nf-wuapi-isearchjob-requestabort
func (j *ISearchJob) RequestAbort() error {
	_, err := oleutil.CallMethod(j.disp, "RequestAbort")
	return err
}
