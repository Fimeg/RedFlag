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

// IWindowsUpdateAgentInfo retrieves information about the version of Windows Update Agent (WUA).
// https://learn.microsoft.com/en-us/windows/win32/api/wuapi/nn-wuapi-iwindowsupdateagentinfo
type IWindowsUpdateAgentInfo struct {
	disp *ole.IDispatch
}

// NewWindowsUpdateAgentInfo creates a new IWindowsUpdateAgentInfo instance.
func NewWindowsUpdateAgentInfo() (*IWindowsUpdateAgentInfo, error) {
	unknown, err := oleutil.CreateObject("Microsoft.Update.AgentInfo")
	if err != nil {
		return nil, err
	}

	disp, err := unknown.QueryInterface(ole.IID_IDispatch)
	if err != nil {
		return nil, err
	}

	return &IWindowsUpdateAgentInfo{disp: disp}, nil
}

// GetInfo retrieves version information about Windows Update Agent.
// varInfoIdentifier can be one of: "ApiMajorVersion", "ApiMinorVersion", "ProductVersionString"
// https://learn.microsoft.com/en-us/windows/win32/api/wuapi/nf-wuapi-iwindowsupdateagentinfo-getinfo
func (info *IWindowsUpdateAgentInfo) GetInfo(varInfoIdentifier string) (interface{}, error) {
	result, err := oleutil.CallMethod(info.disp, "GetInfo", varInfoIdentifier)
	if err != nil {
		return nil, err
	}
	return result.Value(), nil
}

// GetApiMajorVersion returns the major version of the WUA API.
func (info *IWindowsUpdateAgentInfo) GetApiMajorVersion() (int32, error) {
	result, err := info.GetInfo("ApiMajorVersion")
	if err != nil {
		return 0, err
	}
	// Convert result to int32
	switch v := result.(type) {
	case int32:
		return v, nil
	case int:
		return int32(v), nil
	case int64:
		return int32(v), nil
	default:
		return 0, nil
	}
}

// GetApiMinorVersion returns the minor version of the WUA API.
func (info *IWindowsUpdateAgentInfo) GetApiMinorVersion() (int32, error) {
	result, err := info.GetInfo("ApiMinorVersion")
	if err != nil {
		return 0, err
	}
	// Convert result to int32
	switch v := result.(type) {
	case int32:
		return v, nil
	case int:
		return int32(v), nil
	case int64:
		return int32(v), nil
	default:
		return 0, nil
	}
}

// GetProductVersionString returns the product version string of WUA.
func (info *IWindowsUpdateAgentInfo) GetProductVersionString() (string, error) {
	result, err := info.GetInfo("ProductVersionString")
	if err != nil {
		return "", err
	}
	if str, ok := result.(string); ok {
		return str, nil
	}
	return "", nil
}
