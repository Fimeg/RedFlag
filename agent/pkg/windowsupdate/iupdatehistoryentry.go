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

// IUpdateHistoryEntry represents the recorded history of an update.
// https://docs.microsoft.com/en-us/windows/win32/api/wuapi/nn-wuapi-iupdatehistoryentry
type IUpdateHistoryEntry struct {
	disp                *ole.IDispatch
	ClientApplicationID string
	Date                *time.Time
	Description         string
	HResult             int32
	Operation           int32 // enum https://docs.microsoft.com/en-us/windows/win32/api/wuapi/ne-wuapi-updateoperation
	ResultCode          int32 // enum https://docs.microsoft.com/en-us/windows/win32/api/wuapi/ne-wuapi-operationresultcode
	ServerSelection     int32 // enum
	ServiceID           string
	SupportUrl          string
	Title               string
	UninstallationNotes string
	UninstallationSteps []string
	UnmappedResultCode  int32
	UpdateIdentity      *IUpdateIdentity
}

func toIUpdateHistoryEntries(updateHistoryEntriesDisp *ole.IDispatch) ([]*IUpdateHistoryEntry, error) {
	count, err := toInt32Err(oleutil.GetProperty(updateHistoryEntriesDisp, "Count"))
	if err != nil {
		return nil, err
	}

	updateHistoryEntries := make([]*IUpdateHistoryEntry, 0, count)
	for i := 0; i < int(count); i++ {
		updateHistoryEntryDisp, err := toIDispatchErr(oleutil.GetProperty(updateHistoryEntriesDisp, "Item", i))
		if err != nil {
			return nil, err
		}

		updateHistoryEntry, err := toIUpdateHistoryEntry(updateHistoryEntryDisp)
		if err != nil {
			return nil, err
		}

		updateHistoryEntries = append(updateHistoryEntries, updateHistoryEntry)
	}
	return updateHistoryEntries, nil
}

func toIUpdateHistoryEntry(updateHistoryEntryDisp *ole.IDispatch) (*IUpdateHistoryEntry, error) {
	var err error
	iUpdateHistoryEntry := &IUpdateHistoryEntry{
		disp: updateHistoryEntryDisp,
	}

	if iUpdateHistoryEntry.ClientApplicationID, err = toStringErr(oleutil.GetProperty(updateHistoryEntryDisp, "ClientApplicationID")); err != nil {
		return nil, err
	}

	if iUpdateHistoryEntry.Date, err = toTimeErr(oleutil.GetProperty(updateHistoryEntryDisp, "Date")); err != nil {
		return nil, err
	}

	if iUpdateHistoryEntry.Description, err = toStringErr(oleutil.GetProperty(updateHistoryEntryDisp, "Description")); err != nil {
		return nil, err
	}

	if iUpdateHistoryEntry.HResult, err = toInt32Err(oleutil.GetProperty(updateHistoryEntryDisp, "HResult")); err != nil {
		return nil, err
	}

	if iUpdateHistoryEntry.Operation, err = toInt32Err(oleutil.GetProperty(updateHistoryEntryDisp, "Operation")); err != nil {
		return nil, err
	}

	if iUpdateHistoryEntry.ResultCode, err = toInt32Err(oleutil.GetProperty(updateHistoryEntryDisp, "ResultCode")); err != nil {
		return nil, err
	}

	if iUpdateHistoryEntry.ServerSelection, err = toInt32Err(oleutil.GetProperty(updateHistoryEntryDisp, "ServerSelection")); err != nil {
		return nil, err
	}

	if iUpdateHistoryEntry.ServiceID, err = toStringErr(oleutil.GetProperty(updateHistoryEntryDisp, "ServiceID")); err != nil {
		return nil, err
	}

	if iUpdateHistoryEntry.SupportUrl, err = toStringErr(oleutil.GetProperty(updateHistoryEntryDisp, "SupportUrl")); err != nil {
		return nil, err
	}

	if iUpdateHistoryEntry.Title, err = toStringErr(oleutil.GetProperty(updateHistoryEntryDisp, "Title")); err != nil {
		return nil, err
	}

	if iUpdateHistoryEntry.UninstallationNotes, err = toStringErr(oleutil.GetProperty(updateHistoryEntryDisp, "UninstallationNotes")); err != nil {
		return nil, err
	}

	if iUpdateHistoryEntry.UninstallationSteps, err = iStringCollectionToStringArrayErr(toIDispatchErr(oleutil.GetProperty(updateHistoryEntryDisp, "UninstallationSteps"))); err != nil {
		return nil, err
	}

	if iUpdateHistoryEntry.UnmappedResultCode, err = toInt32Err(oleutil.GetProperty(updateHistoryEntryDisp, "UnmappedResultCode")); err != nil {
		return nil, err
	}

	updateIdentityDisp, err := toIDispatchErr(oleutil.GetProperty(updateHistoryEntryDisp, "UpdateIdentity"))
	if err != nil {
		return nil, err
	}
	if updateIdentityDisp != nil {
		if iUpdateHistoryEntry.UpdateIdentity, err = toIUpdateIdentity(updateIdentityDisp); err != nil {
			return nil, err
		}
	}

	return iUpdateHistoryEntry, nil
}
