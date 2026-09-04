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

// IInstallationJob contains properties and methods that are available to an installation operation.
// https://learn.microsoft.com/en-us/windows/win32/api/wuapi/nn-wuapi-iinstallationjob
type IInstallationJob struct {
	disp        *ole.IDispatch
	AsyncState  interface{}
	IsCompleted bool
	Updates     []*IUpdate
}

func toIInstallationJob(disp *ole.IDispatch) (*IInstallationJob, error) {
	if disp == nil {
		return nil, nil
	}

	var err error
	j := &IInstallationJob{disp: disp}

	if j.IsCompleted, err = toBoolErr(oleutil.GetProperty(disp, "IsCompleted")); err != nil {
		return nil, err
	}

	updatesDisp, err := toIDispatchErr(oleutil.GetProperty(disp, "Updates"))
	if err != nil {
		return nil, err
	}
	if updatesDisp != nil {
		if j.Updates, err = toIUpdates(updatesDisp); err != nil {
			return nil, err
		}
	}

	return j, nil
}

// CleanUp releases the resources held by the installation job.
// https://learn.microsoft.com/en-us/windows/win32/api/wuapi/nf-wuapi-iinstallationjob-cleanup
func (j *IInstallationJob) CleanUp() error {
	_, err := oleutil.CallMethod(j.disp, "CleanUp")
	return err
}

// RequestAbort requests that the installation job be canceled.
// https://learn.microsoft.com/en-us/windows/win32/api/wuapi/nf-wuapi-iinstallationjob-requestabort
func (j *IInstallationJob) RequestAbort() error {
	_, err := oleutil.CallMethod(j.disp, "RequestAbort")
	return err
}

// GetProgress returns the current progress of the installation.
// https://learn.microsoft.com/en-us/windows/win32/api/wuapi/nf-wuapi-iinstallationjob-getprogress
func (j *IInstallationJob) GetProgress() (*IInstallationProgress, error) {
	progressDisp, err := toIDispatchErr(oleutil.CallMethod(j.disp, "GetProgress"))
	if err != nil {
		return nil, err
	}
	return toIInstallationProgress(progressDisp)
}
