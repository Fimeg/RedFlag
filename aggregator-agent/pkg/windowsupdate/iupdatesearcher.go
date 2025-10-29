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
