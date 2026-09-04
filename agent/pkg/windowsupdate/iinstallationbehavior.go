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

// IInstallationBehavior represents the installation and uninstallation options of an update.
// https://docs.microsoft.com/en-us/windows/win32/api/wuapi/nn-wuapi-iinstallationbehavior
type IInstallationBehavior struct {
	disp                        *ole.IDispatch
	CanRequestUserInput         bool
	Impact                      int32 // enum https://docs.microsoft.com/en-us/windows/win32/api/wuapi/ne-wuapi-installationimpact
	RebootBehavior              int32 // enum https://docs.microsoft.com/en-us/windows/win32/api/wuapi/ne-wuapi-installationrebootbehavior
	RequiresNetworkConnectivity bool
}

func toIInstallationBehavior(installationBehaviorDisp *ole.IDispatch) (*IInstallationBehavior, error) {
	var err error
	iInstallationBehavior := &IInstallationBehavior{
		disp: installationBehaviorDisp,
	}

	if iInstallationBehavior.CanRequestUserInput, err = toBoolErr(oleutil.GetProperty(installationBehaviorDisp, "CanRequestUserInput")); err != nil {
		return nil, err
	}

	if iInstallationBehavior.Impact, err = toInt32Err(oleutil.GetProperty(installationBehaviorDisp, "Impact")); err != nil {
		return nil, err
	}

	if iInstallationBehavior.RebootBehavior, err = toInt32Err(oleutil.GetProperty(installationBehaviorDisp, "RebootBehavior")); err != nil {
		return nil, err
	}

	if iInstallationBehavior.RequiresNetworkConnectivity, err = toBoolErr(oleutil.GetProperty(installationBehaviorDisp, "RequiresNetworkConnectivity")); err != nil {
		return nil, err
	}

	return iInstallationBehavior, nil
}
