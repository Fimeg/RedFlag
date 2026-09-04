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

// IAutomaticUpdates contains the functionality of Automatic Updates.
// https://learn.microsoft.com/en-us/windows/win32/api/wuapi/nn-wuapi-iautomaticupdates
type IAutomaticUpdates struct {
	disp           *ole.IDispatch
	ServiceEnabled bool
}

// NewAutomaticUpdates creates a new IAutomaticUpdates instance.
func NewAutomaticUpdates() (*IAutomaticUpdates, error) {
	unknown, err := oleutil.CreateObject("Microsoft.Update.AutoUpdate")
	if err != nil {
		return nil, err
	}

	disp, err := unknown.QueryInterface(ole.IID_IDispatch)
	if err != nil {
		return nil, err
	}

	return toIAutomaticUpdates(disp)
}

func toIAutomaticUpdates(autoUpdatesDisp *ole.IDispatch) (*IAutomaticUpdates, error) {
	var err error
	iAutoUpdates := &IAutomaticUpdates{
		disp: autoUpdatesDisp,
	}

	if iAutoUpdates.ServiceEnabled, err = toBoolErr(oleutil.GetProperty(autoUpdatesDisp, "ServiceEnabled")); err != nil {
		return nil, err
	}

	return iAutoUpdates, nil
}

// DetectNow begins detection of updates.
// The DetectNow method returns immediately without waiting for the detection to complete.
// https://learn.microsoft.com/en-us/windows/win32/api/wuapi/nf-wuapi-iautomaticupdates-detectnow
func (a *IAutomaticUpdates) DetectNow() error {
	_, err := oleutil.CallMethod(a.disp, "DetectNow")
	return err
}

// EnableService enables all the components that Automatic Updates requires.
// https://learn.microsoft.com/en-us/windows/win32/api/wuapi/nf-wuapi-iautomaticupdates-enableservice
func (a *IAutomaticUpdates) EnableService() error {
	_, err := oleutil.CallMethod(a.disp, "EnableService")
	if err != nil {
		return err
	}
	a.ServiceEnabled = true
	return nil
}

// Pause pauses automatic updates.
// https://learn.microsoft.com/en-us/windows/win32/api/wuapi/nf-wuapi-iautomaticupdates-pause
func (a *IAutomaticUpdates) Pause() error {
	_, err := oleutil.CallMethod(a.disp, "Pause")
	return err
}

// Resume restarts automatic updates if paused.
// https://learn.microsoft.com/en-us/windows/win32/api/wuapi/nf-wuapi-iautomaticupdates-resume
func (a *IAutomaticUpdates) Resume() error {
	_, err := oleutil.CallMethod(a.disp, "Resume")
	return err
}

// ShowSettingsDialog displays a dialog box that contains settings for Automatic Updates.
// https://learn.microsoft.com/en-us/windows/win32/api/wuapi/nf-wuapi-iautomaticupdates-showsettingsdialog
func (a *IAutomaticUpdates) ShowSettingsDialog() error {
	_, err := oleutil.CallMethod(a.disp, "ShowSettingsDialog")
	return err
}

// GetSettings retrieves the configuration settings for Automatic Updates.
// https://learn.microsoft.com/en-us/windows/win32/api/wuapi/nf-wuapi-iautomaticupdates-get_settings
func (a *IAutomaticUpdates) GetSettings() (*IAutomaticUpdatesSettings, error) {
	settingsDisp, err := toIDispatchErr(oleutil.GetProperty(a.disp, "Settings"))
	if err != nil {
		return nil, err
	}
	return toIAutomaticUpdatesSettings(settingsDisp)
}

// GetResults retrieves the results of the last Automatic Updates search. (IAutomaticUpdates2)
// https://learn.microsoft.com/en-us/windows/win32/api/wuapi/nf-wuapi-iautomaticupdates2-get_results
func (a *IAutomaticUpdates) GetResults() (*IAutomaticUpdatesResults, error) {
	resultsDisp, err := toIDispatchErr(oleutil.GetProperty(a.disp, "Results"))
	if err != nil {
		return nil, err
	}
	return toIAutomaticUpdatesResults(resultsDisp)
}

// IAutomaticUpdatesResults contains the read-only properties that describe Automatic Updates.
// https://learn.microsoft.com/en-us/windows/win32/api/wuapi/nn-wuapi-iautomaticupdatesresults
type IAutomaticUpdatesResults struct {
	disp                        *ole.IDispatch
	LastInstallationSuccessDate *time.Time
	LastSearchSuccessDate       *time.Time
}

func toIAutomaticUpdatesResults(disp *ole.IDispatch) (*IAutomaticUpdatesResults, error) {
	if disp == nil {
		return nil, nil
	}

	var err error
	r := &IAutomaticUpdatesResults{disp: disp}

	if r.LastInstallationSuccessDate, err = toTimeErr(oleutil.GetProperty(disp, "LastInstallationSuccessDate")); err != nil {
		return nil, err
	}

	if r.LastSearchSuccessDate, err = toTimeErr(oleutil.GetProperty(disp, "LastSearchSuccessDate")); err != nil {
		return nil, err
	}

	return r, nil
}
