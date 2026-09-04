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

// IInstallationProgress represents the progress of an asynchronous installation operation.
// https://learn.microsoft.com/en-us/windows/win32/api/wuapi/nn-wuapi-iinstallationprogress
type IInstallationProgress struct {
	disp                         *ole.IDispatch
	CurrentUpdateIndex           int32
	CurrentUpdatePercentComplete int32
	PercentComplete              int32
}

func toIInstallationProgress(disp *ole.IDispatch) (*IInstallationProgress, error) {
	if disp == nil {
		return nil, nil
	}

	var err error
	p := &IInstallationProgress{disp: disp}

	if p.CurrentUpdateIndex, err = toInt32Err(oleutil.GetProperty(disp, "CurrentUpdateIndex")); err != nil {
		return nil, err
	}

	if p.CurrentUpdatePercentComplete, err = toInt32Err(oleutil.GetProperty(disp, "CurrentUpdatePercentComplete")); err != nil {
		return nil, err
	}

	if p.PercentComplete, err = toInt32Err(oleutil.GetProperty(disp, "PercentComplete")); err != nil {
		return nil, err
	}

	return p, nil
}

// GetUpdateResult returns the result of the installation for a specified update.
// https://learn.microsoft.com/en-us/windows/win32/api/wuapi/nf-wuapi-iinstallationprogress-getupdateresult
func (p *IInstallationProgress) GetUpdateResult(updateIndex int32) (*IUpdateInstallationResult, error) {
	resultDisp, err := toIDispatchErr(oleutil.CallMethod(p.disp, "GetUpdateResult", updateIndex))
	if err != nil {
		return nil, err
	}
	return toIUpdateInstallationResult(resultDisp)
}
