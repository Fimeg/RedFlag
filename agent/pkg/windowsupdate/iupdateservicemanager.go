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

// IUpdateServiceManager adds or removes the registration of an update service with Windows Update Agent.
// https://learn.microsoft.com/en-us/windows/win32/api/wuapi/nn-wuapi-iupdateservicemanager
type IUpdateServiceManager struct {
	disp     *ole.IDispatch
	Services []*IUpdateService
	// IUpdateServiceManager2 properties
	ClientApplicationID string
}

// NewUpdateServiceManager creates a new IUpdateServiceManager instance.
func NewUpdateServiceManager() (*IUpdateServiceManager, error) {
	unknown, err := oleutil.CreateObject("Microsoft.Update.ServiceManager")
	if err != nil {
		return nil, err
	}

	disp, err := unknown.QueryInterface(ole.IID_IDispatch)
	if err != nil {
		return nil, err
	}

	return toIUpdateServiceManager(disp)
}

func toIUpdateServiceManager(disp *ole.IDispatch) (*IUpdateServiceManager, error) {
	if disp == nil {
		return nil, nil
	}

	sm := &IUpdateServiceManager{disp: disp}

	servicesDisp, err := toIDispatchErr(oleutil.GetProperty(disp, "Services"))
	if err == nil && servicesDisp != nil {
		services, err := toIUpdateServices(servicesDisp)
		if err == nil {
			sm.Services = services
		} else {
			// If we can't parse services, initialize to empty slice instead of nil
			sm.Services = make([]*IUpdateService, 0)
		}
	} else {
		// If Services property doesn't exist or fails, initialize to empty slice
		sm.Services = make([]*IUpdateService, 0)
	}

	// IUpdateServiceManager2 property
	if clientAppID, err := toStringErr(oleutil.GetProperty(disp, "ClientApplicationID")); err == nil {
		sm.ClientApplicationID = clientAppID
	}

	return sm, nil
}

// AddService registers a service with Windows Update Agent (WUA).
// https://learn.microsoft.com/en-us/windows/win32/api/wuapi/nf-wuapi-iupdateservicemanager-addservice
func (sm *IUpdateServiceManager) AddService(serviceID string, authorizationCabPath string) (*IUpdateService, error) {
	serviceDisp, err := toIDispatchErr(oleutil.CallMethod(sm.disp, "AddService", serviceID, authorizationCabPath))
	if err != nil {
		return nil, err
	}
	return toIUpdateService(serviceDisp)
}

// RegisterServiceWithAU registers a service with Automatic Updates.
// https://learn.microsoft.com/en-us/windows/win32/api/wuapi/nf-wuapi-iupdateservicemanager-registerservicewithau
func (sm *IUpdateServiceManager) RegisterServiceWithAU(serviceID string) error {
	_, err := oleutil.CallMethod(sm.disp, "RegisterServiceWithAU", serviceID)
	return err
}

// RemoveService removes a service registration from Windows Update Agent (WUA).
// https://learn.microsoft.com/en-us/windows/win32/api/wuapi/nf-wuapi-iupdateservicemanager-removeservice
func (sm *IUpdateServiceManager) RemoveService(serviceID string) error {
	_, err := oleutil.CallMethod(sm.disp, "RemoveService", serviceID)
	return err
}

// UnregisterServiceWithAU unregisters a service with Automatic Updates.
// https://learn.microsoft.com/en-us/windows/win32/api/wuapi/nf-wuapi-iupdateservicemanager-unregisterservicewithau
func (sm *IUpdateServiceManager) UnregisterServiceWithAU(serviceID string) error {
	_, err := oleutil.CallMethod(sm.disp, "UnregisterServiceWithAU", serviceID)
	return err
}

// SetOption sets options for the update service manager.
// https://learn.microsoft.com/en-us/windows/win32/api/wuapi/nf-wuapi-iupdateservicemanager-setoption
func (sm *IUpdateServiceManager) SetOption(optionName string, optionValue interface{}) error {
	_, err := oleutil.CallMethod(sm.disp, "SetOption", optionName, optionValue)
	return err
}

// AddScanPackageService registers a scan package service.
// https://learn.microsoft.com/en-us/windows/win32/api/wuapi/nf-wuapi-iupdateservicemanager-addscanpackageservice
func (sm *IUpdateServiceManager) AddScanPackageService(serviceName string, scanFileLocation string, flags int32) (*IUpdateService, error) {
	serviceDisp, err := toIDispatchErr(oleutil.CallMethod(sm.disp, "AddScanPackageService", serviceName, scanFileLocation, flags))
	if err != nil {
		return nil, err
	}
	return toIUpdateService(serviceDisp)
}

// PutClientApplicationID sets the identifier of the current client application. (IUpdateServiceManager2)
// https://learn.microsoft.com/en-us/windows/win32/api/wuapi/nf-wuapi-iupdateservicemanager2-put_clientapplicationid
func (sm *IUpdateServiceManager) PutClientApplicationID(value string) error {
	_, err := oleutil.PutProperty(sm.disp, "ClientApplicationID", value)
	if err != nil {
		return err
	}
	sm.ClientApplicationID = value
	return nil
}

// AddService2 registers a service with Windows Update Agent (WUA). (IUpdateServiceManager2)
// https://learn.microsoft.com/en-us/windows/win32/api/wuapi/nf-wuapi-iupdateservicemanager2-addservice2
func (sm *IUpdateServiceManager) AddService2(serviceID string, flags int32, authorizationCabPath string) (*IUpdateServiceRegistration, error) {
	regDisp, err := toIDispatchErr(oleutil.CallMethod(sm.disp, "AddService2", serviceID, flags, authorizationCabPath))
	if err != nil {
		return nil, err
	}
	return toIUpdateServiceRegistration(regDisp)
}
