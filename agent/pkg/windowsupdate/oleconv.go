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
)

func toIDispatchErr(result *ole.VARIANT, err error) (*ole.IDispatch, error) {
	if err != nil {
		return nil, err
	}
	return variantToIDispatch(result), nil
}

func toInt64Err(result *ole.VARIANT, err error) (int64, error) {
	if err != nil {
		return 0, err
	}
	return variantToInt64(result), nil
}

func toInt32Err(result *ole.VARIANT, err error) (int32, error) {
	if err != nil {
		return 0, err
	}
	return variantToInt32(result), nil
}

func toFloat64Err(result *ole.VARIANT, err error) (float64, error) {
	if err != nil {
		return 0, err
	}
	return variantToFloat64(result), nil
}

func toFloat32Err(result *ole.VARIANT, err error) (float32, error) {
	if err != nil {
		return 0, err
	}
	return variantToFloat32(result), nil
}

func toStringErr(result *ole.VARIANT, err error) (string, error) {
	if err != nil {
		return "", err
	}
	return variantToString(result), nil
}

func toBoolErr(result *ole.VARIANT, err error) (bool, error) {
	if err != nil {
		return false, err
	}
	return variantToBool(result), nil
}

func toTimeErr(result *ole.VARIANT, err error) (*time.Time, error) {
	if err != nil {
		return nil, err
	}
	return variantToTime(result), nil
}

func variantToIDispatch(v *ole.VARIANT) *ole.IDispatch {
	value := v.Value()
	if value == nil {
		return nil
	}
	return v.ToIDispatch()
}

func variantToInt64(v *ole.VARIANT) int64 {
	value := v.Value()
	if value == nil {
		return 0
	}
	return value.(int64)
}

func variantToInt32(v *ole.VARIANT) int32 {
	value := v.Value()
	if value == nil {
		return 0
	}
	return value.(int32)
}

func variantToFloat64(v *ole.VARIANT) float64 {
	value := v.Value()
	if value == nil {
		return 0
	}
	return value.(float64)
}

func variantToFloat32(v *ole.VARIANT) float32 {
	value := v.Value()
	if value == nil {
		return 0
	}
	return value.(float32)
}

func variantToString(v *ole.VARIANT) string {
	value := v.Value()
	if value == nil {
		return ""
	}
	return value.(string)
}

func variantToBool(v *ole.VARIANT) bool {
	value := v.Value()
	if value == nil {
		return false
	}
	return value.(bool)
}

func variantToTime(v *ole.VARIANT) *time.Time {
	value := v.Value()
	if value == nil {
		return nil
	}
	valueTime := value.(time.Time)
	return &valueTime
}
