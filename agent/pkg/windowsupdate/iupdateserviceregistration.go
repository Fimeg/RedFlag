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

// IUpdateServiceRegistration represents the registration status of an update service.
// https://learn.microsoft.com/en-us/windows/win32/api/wuapi/nn-wuapi-iupdateserviceregistration
type IUpdateServiceRegistration struct {
	disp                        *ole.IDispatch
	IsPendingRegistrationWithAU bool
	RegistrationState           int32
	Service                     *IUpdateService
}

func toIUpdateServiceRegistration(disp *ole.IDispatch) (*IUpdateServiceRegistration, error) {
	if disp == nil {
		return nil, nil
	}

	var err error
	reg := &IUpdateServiceRegistration{disp: disp}

	if reg.IsPendingRegistrationWithAU, err = toBoolErr(oleutil.GetProperty(disp, "IsPendingRegistrationWithAU")); err != nil {
		return nil, err
	}

	if reg.RegistrationState, err = toInt32Err(oleutil.GetProperty(disp, "RegistrationState")); err != nil {
		return nil, err
	}

	serviceDisp, err := toIDispatchErr(oleutil.GetProperty(disp, "Service"))
	if err != nil {
		return nil, err
	}
	if serviceDisp != nil {
		if reg.Service, err = toIUpdateService(serviceDisp); err != nil {
			return nil, err
		}
	}

	return reg, nil
}
