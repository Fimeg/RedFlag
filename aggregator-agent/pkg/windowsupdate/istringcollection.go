package windowsupdate

import (
	"github.com/go-ole/go-ole"
	"github.com/go-ole/go-ole/oleutil"
)

// https://docs.microsoft.com/en-us/windows/win32/api/wuapi/nn-wuapi-istringcollection
func iStringCollectionToStringArrayErr(disp *ole.IDispatch, err error) ([]string, error) {
	if err != nil {
		return nil, err
	}

	if disp == nil {
		return nil, nil
	}

	count, err := toInt32Err(oleutil.GetProperty(disp, "Count"))
	if err != nil {
		return nil, err
	}

	stringCollection := make([]string, count)

	for i := 0; i < int(count); i++ {
		str, err := toStringErr(oleutil.GetProperty(disp, "Item", i))
		if err != nil {
			return nil, err
		}

		stringCollection[i] = str
	}
	return stringCollection, nil
}
