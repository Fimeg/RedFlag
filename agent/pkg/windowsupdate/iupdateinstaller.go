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

// IUpdateInstaller installs or uninstalls updates from or onto a computer.
// https://docs.microsoft.com/en-us/windows/win32/api/wuapi/nn-wuapi-iupdateinstaller
type IUpdateInstaller struct {
	disp                *ole.IDispatch
	AllowSourcePrompts  bool
	ClientApplicationID string
	IsBusy              bool
	IsForced            bool
	ForceQuiet          bool
	// ParentHwnd                       HWND
	// ParentWindow                     IUnknown
	RebootRequiredBeforeInstallation bool
	Updates                          []*IUpdate
}

func toIUpdateInstaller(updateInstallerDisp *ole.IDispatch) (*IUpdateInstaller, error) {
	var err error
	iUpdateInstaller := &IUpdateInstaller{
		disp: updateInstallerDisp,
	}

	if iUpdateInstaller.AllowSourcePrompts, err = toBoolErr(oleutil.GetProperty(updateInstallerDisp, "AllowSourcePrompts")); err != nil {
		return nil, err
	}

	if iUpdateInstaller.ClientApplicationID, err = toStringErr(oleutil.GetProperty(updateInstallerDisp, "ClientApplicationID")); err != nil {
		return nil, err
	}

	if iUpdateInstaller.IsBusy, err = toBoolErr(oleutil.GetProperty(updateInstallerDisp, "IsBusy")); err != nil {
		return nil, err
	}

	if iUpdateInstaller.IsForced, err = toBoolErr(oleutil.GetProperty(updateInstallerDisp, "IsForced")); err != nil {
		return nil, err
	}

	if iUpdateInstaller.ForceQuiet, err = toBoolErr(oleutil.GetProperty(updateInstallerDisp, "ForceQuiet")); err != nil {
		return nil, err
	}

	if iUpdateInstaller.RebootRequiredBeforeInstallation, err = toBoolErr(oleutil.GetProperty(updateInstallerDisp, "RebootRequiredBeforeInstallation")); err != nil {
		return nil, err
	}

	updatesDisp, err := toIDispatchErr(oleutil.GetProperty(updateInstallerDisp, "Updates"))
	if err != nil {
		return nil, err
	}
	if updatesDisp != nil {
		if iUpdateInstaller.Updates, err = toIUpdates(updatesDisp); err != nil {
			return nil, err
		}
	}

	return iUpdateInstaller, nil
}

// Install starts a synchronous installation of the updates.
// https://docs.microsoft.com/en-us/windows/win32/api/wuapi/nf-wuapi-iupdateinstaller-install
func (iUpdateInstaller *IUpdateInstaller) Install(updates []*IUpdate) (*IInstallationResult, error) {
	updatesDisp, err := toIUpdateCollection(updates)
	if err != nil {
		return nil, err
	}
	if _, err = oleutil.PutProperty(iUpdateInstaller.disp, "Updates", updatesDisp); err != nil {
		return nil, err
	}

	installationResultDisp, err := toIDispatchErr(oleutil.CallMethod(iUpdateInstaller.disp, "Install"))
	if err != nil {
		return nil, err
	}
	return toIInstallationResult(installationResultDisp)
}

// Finalizes updates that were previously staged or installed.
// https://learn.microsoft.com/en-us/windows/win32/api/wuapi/nf-wuapi-iupdateinstaller4-commit
func (iUpdateInstaller *IUpdateInstaller) Commit(dwFlags int32) error {
	_, err := toIDispatchErr(oleutil.CallMethod(iUpdateInstaller.disp, "Commit", dwFlags))
	if err != nil {
		return err
	}
	return nil
}

// Sets a Boolean value that indicates whether Windows Installer is forced to install the updates without user interaction.
// https://learn.microsoft.com/en-us/windows/win32/api/wuapi/nf-wuapi-iupdateinstaller2-put_forcequiet
func (iUpdateInstaller *IUpdateInstaller) PutForceQuiet(value bool) error {
	_, err := toIDispatchErr(oleutil.PutProperty(iUpdateInstaller.disp, "ForceQuiet", value))
	if err != nil {
		return err
	}
	iUpdateInstaller.ForceQuiet = value
	return nil
}

// Sets a Boolean value that indicates whether to forcibly install or uninstall an update.
// https://learn.microsoft.com/en-us/windows/win32/api/wuapi/nf-wuapi-iupdateinstaller-put_isforced
func (iUpdateInstaller *IUpdateInstaller) PutIsForced(value bool) error {
	_, err := toIDispatchErr(oleutil.PutProperty(iUpdateInstaller.disp, "IsForced", value))
	if err != nil {
		return err
	}
	iUpdateInstaller.IsForced = value
	return nil
}

// Uninstall starts a synchronous uninstallation of the updates.
// https://learn.microsoft.com/en-us/windows/win32/api/wuapi/nf-wuapi-iupdateinstaller-uninstall
func (iUpdateInstaller *IUpdateInstaller) Uninstall(updates []*IUpdate) (*IInstallationResult, error) {
	updatesDisp, err := toIUpdateCollection(updates)
	if err != nil {
		return nil, err
	}
	if _, err = oleutil.PutProperty(iUpdateInstaller.disp, "Updates", updatesDisp); err != nil {
		return nil, err
	}

	uninstallationResultDisp, err := toIDispatchErr(oleutil.CallMethod(iUpdateInstaller.disp, "Uninstall"))
	if err != nil {
		return nil, err
	}
	return toIInstallationResult(uninstallationResultDisp)
}

// BeginInstall begins an asynchronous installation of the updates.
// https://learn.microsoft.com/en-us/windows/win32/api/wuapi/nf-wuapi-iupdateinstaller-begininstall
func (iUpdateInstaller *IUpdateInstaller) BeginInstall(updates []*IUpdate) (*IInstallationJob, error) {
	updatesDisp, err := toIUpdateCollection(updates)
	if err != nil {
		return nil, err
	}
	if _, err = oleutil.PutProperty(iUpdateInstaller.disp, "Updates", updatesDisp); err != nil {
		return nil, err
	}

	jobDisp, err := toIDispatchErr(oleutil.CallMethod(iUpdateInstaller.disp, "BeginInstall", nil, nil, nil))
	if err != nil {
		return nil, err
	}
	return toIInstallationJob(jobDisp)
}

// EndInstall completes an asynchronous installation.
// https://learn.microsoft.com/en-us/windows/win32/api/wuapi/nf-wuapi-iupdateinstaller-endinstall
func (iUpdateInstaller *IUpdateInstaller) EndInstall(installationJob *IInstallationJob) (*IInstallationResult, error) {
	resultDisp, err := toIDispatchErr(oleutil.CallMethod(iUpdateInstaller.disp, "EndInstall", installationJob.disp))
	if err != nil {
		return nil, err
	}
	return toIInstallationResult(resultDisp)
}

// BeginUninstall begins an asynchronous uninstallation of the updates.
// https://learn.microsoft.com/en-us/windows/win32/api/wuapi/nf-wuapi-iupdateinstaller-beginuninstall
func (iUpdateInstaller *IUpdateInstaller) BeginUninstall(updates []*IUpdate) (*IInstallationJob, error) {
	updatesDisp, err := toIUpdateCollection(updates)
	if err != nil {
		return nil, err
	}
	if _, err = oleutil.PutProperty(iUpdateInstaller.disp, "Updates", updatesDisp); err != nil {
		return nil, err
	}

	jobDisp, err := toIDispatchErr(oleutil.CallMethod(iUpdateInstaller.disp, "BeginUninstall", nil, nil, nil))
	if err != nil {
		return nil, err
	}
	return toIInstallationJob(jobDisp)
}

// EndUninstall completes an asynchronous uninstallation.
// https://learn.microsoft.com/en-us/windows/win32/api/wuapi/nf-wuapi-iupdateinstaller-enduninstall
func (iUpdateInstaller *IUpdateInstaller) EndUninstall(installationJob *IInstallationJob) (*IInstallationResult, error) {
	resultDisp, err := toIDispatchErr(oleutil.CallMethod(iUpdateInstaller.disp, "EndUninstall", installationJob.disp))
	if err != nil {
		return nil, err
	}
	return toIInstallationResult(resultDisp)
}

// PutAllowSourcePrompts sets whether prompts are allowed during installation.
func (iUpdateInstaller *IUpdateInstaller) PutAllowSourcePrompts(value bool) error {
	_, err := oleutil.PutProperty(iUpdateInstaller.disp, "AllowSourcePrompts", value)
	if err != nil {
		return err
	}
	iUpdateInstaller.AllowSourcePrompts = value
	return nil
}

// PutClientApplicationID sets the identifier of the current client application.
func (iUpdateInstaller *IUpdateInstaller) PutClientApplicationID(value string) error {
	_, err := oleutil.PutProperty(iUpdateInstaller.disp, "ClientApplicationID", value)
	if err != nil {
		return err
	}
	iUpdateInstaller.ClientApplicationID = value
	return nil
}
