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

// IUpdateSearcher searches for updates on a server.
// https://docs.microsoft.com/en-us/windows/win32/api/wuapi/nn-wuapi-iupdatesearcher
type IUpdateSearcher struct {
	disp                                *ole.IDispatch
	CanAutomaticallyUpgradeService      bool
	ClientApplicationID                 string
	IncludePotentiallySupersededUpdates bool
	Online                              bool
	ServerSelection                     int32
	ServiceID                           string
}

func toIUpdateSearcher(updateSearcherDisp *ole.IDispatch) (*IUpdateSearcher, error) {
	var err error
	iUpdateSearcher := &IUpdateSearcher{
		disp: updateSearcherDisp,
	}

	if iUpdateSearcher.CanAutomaticallyUpgradeService, err = toBoolErr(oleutil.GetProperty(updateSearcherDisp, "CanAutomaticallyUpgradeService")); err != nil {
		return nil, err
	}

	if iUpdateSearcher.ClientApplicationID, err = toStringErr(oleutil.GetProperty(updateSearcherDisp, "ClientApplicationID")); err != nil {
		return nil, err
	}

	if iUpdateSearcher.IncludePotentiallySupersededUpdates, err = toBoolErr(oleutil.GetProperty(updateSearcherDisp, "IncludePotentiallySupersededUpdates")); err != nil {
		return nil, err
	}

	if iUpdateSearcher.Online, err = toBoolErr(oleutil.GetProperty(updateSearcherDisp, "Online")); err != nil {
		return nil, err
	}

	if iUpdateSearcher.ServerSelection, err = toInt32Err(oleutil.GetProperty(updateSearcherDisp, "ServerSelection")); err != nil {
		return nil, err
	}

	if iUpdateSearcher.ServiceID, err = toStringErr(oleutil.GetProperty(updateSearcherDisp, "ServiceID")); err != nil {
		return nil, err
	}

	return iUpdateSearcher, nil
}

// Search performs a synchronous search for updates. The search uses the search options that are currently configured.
// https://docs.microsoft.com/en-us/windows/win32/api/wuapi/nf-wuapi-iupdatesearcher-search
func (iUpdateSearcher *IUpdateSearcher) Search(criteria string) (*ISearchResult, error) {
	searchResultDisp, err := toIDispatchErr(oleutil.CallMethod(iUpdateSearcher.disp, "Search", criteria))
	if err != nil {
		return nil, err
	}
	return toISearchResult(searchResultDisp)
}

// QueryHistory synchronously queries the computer for the history of the update events.
// https://docs.microsoft.com/en-us/windows/win32/api/wuapi/nf-wuapi-iupdatesearcher-queryhistory
func (iUpdateSearcher *IUpdateSearcher) QueryHistory(startIndex int32, count int32) ([]*IUpdateHistoryEntry, error) {
	updateHistoryEntriesDisp, err := toIDispatchErr(oleutil.CallMethod(iUpdateSearcher.disp, "QueryHistory", startIndex, count))
	if err != nil {
		return nil, err
	}
	return toIUpdateHistoryEntries(updateHistoryEntriesDisp)
}

// GetTotalHistoryCount returns the number of update events on the computer.
// https://docs.microsoft.com/en-us/windows/win32/api/wuapi/nf-wuapi-iupdatesearcher-gettotalhistorycount
func (iUpdateSearcher *IUpdateSearcher) GetTotalHistoryCount() (int32, error) {
	// According to MSDN, this is a method with an [out, retval] parameter
	// In COM automation through IDispatch, such methods return the value directly
	return toInt32Err(oleutil.CallMethod(iUpdateSearcher.disp, "GetTotalHistoryCount"))
}

// QueryHistoryAll synchronously queries the computer for the history of all update events.
func (iUpdateSearcher *IUpdateSearcher) QueryHistoryAll() ([]*IUpdateHistoryEntry, error) {
	count, err := iUpdateSearcher.GetTotalHistoryCount()
	if err != nil {
		return nil, err
	}
	return iUpdateSearcher.QueryHistory(0, count)
}

// BeginSearch begins an asynchronous search for updates.
// https://learn.microsoft.com/en-us/windows/win32/api/wuapi/nf-wuapi-iupdatesearcher-beginsearch
func (iUpdateSearcher *IUpdateSearcher) BeginSearch(criteria string) (*ISearchJob, error) {
	jobDisp, err := toIDispatchErr(oleutil.CallMethod(iUpdateSearcher.disp, "BeginSearch", criteria, nil, nil))
	if err != nil {
		return nil, err
	}
	return toISearchJob(jobDisp)
}

// EndSearch completes an asynchronous search.
// https://learn.microsoft.com/en-us/windows/win32/api/wuapi/nf-wuapi-iupdatesearcher-endsearch
func (iUpdateSearcher *IUpdateSearcher) EndSearch(searchJob *ISearchJob) (*ISearchResult, error) {
	resultDisp, err := toIDispatchErr(oleutil.CallMethod(iUpdateSearcher.disp, "EndSearch", searchJob.disp))
	if err != nil {
		return nil, err
	}
	return toISearchResult(resultDisp)
}

// EscapeString converts a string into a string that can be used as a literal value in a search criteria string.
// https://learn.microsoft.com/en-us/windows/win32/api/wuapi/nf-wuapi-iupdatesearcher-escapestring
func (iUpdateSearcher *IUpdateSearcher) EscapeString(unescaped string) (string, error) {
	return toStringErr(oleutil.CallMethod(iUpdateSearcher.disp, "EscapeString", unescaped))
}

// PutClientApplicationID sets the identifier of the current client application.
func (iUpdateSearcher *IUpdateSearcher) PutClientApplicationID(value string) error {
	_, err := oleutil.PutProperty(iUpdateSearcher.disp, "ClientApplicationID", value)
	if err != nil {
		return err
	}
	iUpdateSearcher.ClientApplicationID = value
	return nil
}

// PutServerSelection sets the server to search.
func (iUpdateSearcher *IUpdateSearcher) PutServerSelection(value int32) error {
	_, err := oleutil.PutProperty(iUpdateSearcher.disp, "ServerSelection", value)
	if err != nil {
		return err
	}
	iUpdateSearcher.ServerSelection = value
	return nil
}

// PutServiceID sets the ServiceID to search.
func (iUpdateSearcher *IUpdateSearcher) PutServiceID(value string) error {
	_, err := oleutil.PutProperty(iUpdateSearcher.disp, "ServiceID", value)
	if err != nil {
		return err
	}
	iUpdateSearcher.ServiceID = value
	return nil
}

// PutOnline sets whether to search online.
func (iUpdateSearcher *IUpdateSearcher) PutOnline(value bool) error {
	_, err := oleutil.PutProperty(iUpdateSearcher.disp, "Online", value)
	if err != nil {
		return err
	}
	iUpdateSearcher.Online = value
	return nil
}

// PutIncludePotentiallySupersededUpdates sets whether to include potentially superseded updates.
func (iUpdateSearcher *IUpdateSearcher) PutIncludePotentiallySupersededUpdates(value bool) error {
	_, err := oleutil.PutProperty(iUpdateSearcher.disp, "IncludePotentiallySupersededUpdates", value)
	if err != nil {
		return err
	}
	iUpdateSearcher.IncludePotentiallySupersededUpdates = value
	return nil
}
