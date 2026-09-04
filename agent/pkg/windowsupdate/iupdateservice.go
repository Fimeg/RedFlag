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
	"time"

	"github.com/go-ole/go-ole"
	"github.com/go-ole/go-ole/oleutil"
)

// IUpdateService contains information about a service that is registered with Windows Update Agent (WUA).
// https://learn.microsoft.com/en-us/windows/win32/api/wuapi/nn-wuapi-iupdateservice
type IUpdateService struct {
	disp                  *ole.IDispatch
	CanRegisterWithAU     bool
	ContentValidationCert []byte
	ExpirationDate        *time.Time
	IsManaged             bool
	IsRegisteredWithAU    bool
	IsScanPackageService  bool
	IssueDate             *time.Time
	Name                  string
	OffersWindowsUpdates  bool
	RedirectUrls          []string
	ServiceID             string
	ServiceUrl            string
	SetupPrefix           string
	// IUpdateService2 properties
	IsDefaultAUService bool
}

func toIUpdateService(disp *ole.IDispatch) (*IUpdateService, error) {
	if disp == nil {
		return nil, nil
	}

	var err error
	s := &IUpdateService{disp: disp}

	if s.CanRegisterWithAU, err = toBoolErr(oleutil.GetProperty(disp, "CanRegisterWithAU")); err != nil {
		return nil, err
	}

	if s.IsManaged, err = toBoolErr(oleutil.GetProperty(disp, "IsManaged")); err != nil {
		return nil, err
	}

	if s.IsRegisteredWithAU, err = toBoolErr(oleutil.GetProperty(disp, "IsRegisteredWithAU")); err != nil {
		return nil, err
	}

	if s.IsScanPackageService, err = toBoolErr(oleutil.GetProperty(disp, "IsScanPackageService")); err != nil {
		return nil, err
	}

	if s.IssueDate, err = toTimeErr(oleutil.GetProperty(disp, "IssueDate")); err != nil {
		return nil, err
	}

	if s.ExpirationDate, err = toTimeErr(oleutil.GetProperty(disp, "ExpirationDate")); err != nil {
		return nil, err
	}

	if s.Name, err = toStringErr(oleutil.GetProperty(disp, "Name")); err != nil {
		return nil, err
	}

	if s.OffersWindowsUpdates, err = toBoolErr(oleutil.GetProperty(disp, "OffersWindowsUpdates")); err != nil {
		return nil, err
	}

	if s.RedirectUrls, err = iStringCollectionToStringArrayErr(toIDispatchErr(oleutil.GetProperty(disp, "RedirectUrls"))); err != nil {
		return nil, err
	}

	if s.ServiceID, err = toStringErr(oleutil.GetProperty(disp, "ServiceID")); err != nil {
		return nil, err
	}

	if s.ServiceUrl, err = toStringErr(oleutil.GetProperty(disp, "ServiceUrl")); err != nil {
		return nil, err
	}

	if s.SetupPrefix, err = toStringErr(oleutil.GetProperty(disp, "SetupPrefix")); err != nil {
		return nil, err
	}

	// IUpdateService2 property
	if isDefault, err := toBoolErr(oleutil.GetProperty(disp, "IsDefaultAUService")); err == nil {
		s.IsDefaultAUService = isDefault
	}

	return s, nil
}

func toIUpdateServices(disp *ole.IDispatch) ([]*IUpdateService, error) {
	if disp == nil {
		return nil, nil
	}

	count, err := toInt32Err(oleutil.GetProperty(disp, "Count"))
	if err != nil {
		return nil, err
	}

	services := make([]*IUpdateService, 0, count)
	for i := int32(0); i < count; i++ {
		serviceDisp, err := toIDispatchErr(oleutil.GetProperty(disp, "Item", i))
		if err != nil {
			return nil, err
		}

		service, err := toIUpdateService(serviceDisp)
		if err != nil {
			return nil, err
		}

		services = append(services, service)
	}
	return services, nil
}
