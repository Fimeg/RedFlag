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

// IImageInformation contains information about a localized image that is associated with an update or a category.
// https://docs.microsoft.com/en-us/windows/win32/api/wuapi/nn-wuapi-iimageinformation
type IImageInformation struct {
	disp    *ole.IDispatch
	AltText string
	Height  int64
	Source  string
	Width   int64
}

func toIImageInformation(imageInformationDisp *ole.IDispatch) (*IImageInformation, error) {
	var err error
	iImageInformation := &IImageInformation{
		disp: imageInformationDisp,
	}

	if iImageInformation.AltText, err = toStringErr(oleutil.GetProperty(imageInformationDisp, "AltText")); err != nil {
		return nil, err
	}

	if iImageInformation.Height, err = toInt64Err(oleutil.GetProperty(imageInformationDisp, "Height")); err != nil {
		return nil, err
	}

	if iImageInformation.Source, err = toStringErr(oleutil.GetProperty(imageInformationDisp, "Source")); err != nil {
		return nil, err
	}

	if iImageInformation.Width, err = toInt64Err(oleutil.GetProperty(imageInformationDisp, "Width")); err != nil {
		return nil, err
	}

	return iImageInformation, nil
}
