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

// IAutomaticUpdatesSettings contains the settings that are available in Automatic Updates.
// https://learn.microsoft.com/en-us/windows/win32/api/wuapi/nn-wuapi-iautomaticupdatessettings
type IAutomaticUpdatesSettings struct {
	disp                      *ole.IDispatch
	NotificationLevel         int32 // AutomaticUpdatesNotificationLevel enum
	ReadOnly                  bool
	Required                  bool
	ScheduledInstallationDay  int32 // AutomaticUpdatesScheduledInstallationDay enum (not supported on Windows 8+)
	ScheduledInstallationTime int32 // Hour of the day (0-23) (not supported on Windows 8+)
}

func toIAutomaticUpdatesSettings(settingsDisp *ole.IDispatch) (*IAutomaticUpdatesSettings, error) {
	var err error
	iSettings := &IAutomaticUpdatesSettings{
		disp: settingsDisp,
	}

	if iSettings.NotificationLevel, err = toInt32Err(oleutil.GetProperty(settingsDisp, "NotificationLevel")); err != nil {
		return nil, err
	}

	if iSettings.ReadOnly, err = toBoolErr(oleutil.GetProperty(settingsDisp, "ReadOnly")); err != nil {
		return nil, err
	}

	if iSettings.Required, err = toBoolErr(oleutil.GetProperty(settingsDisp, "Required")); err != nil {
		return nil, err
	}

	if iSettings.ScheduledInstallationDay, err = toInt32Err(oleutil.GetProperty(settingsDisp, "ScheduledInstallationDay")); err != nil {
		return nil, err
	}

	if iSettings.ScheduledInstallationTime, err = toInt32Err(oleutil.GetProperty(settingsDisp, "ScheduledInstallationTime")); err != nil {
		return nil, err
	}

	return iSettings, nil
}

// Refresh reads the latest Automatic Updates settings.
// https://learn.microsoft.com/en-us/windows/win32/api/wuapi/nf-wuapi-iautomaticupdatessettings-refresh
func (s *IAutomaticUpdatesSettings) Refresh() error {
	_, err := oleutil.CallMethod(s.disp, "Refresh")
	if err != nil {
		return err
	}

	// Re-read properties after refresh
	refreshed, err := toIAutomaticUpdatesSettings(s.disp)
	if err != nil {
		return err
	}

	s.NotificationLevel = refreshed.NotificationLevel
	s.ReadOnly = refreshed.ReadOnly
	s.Required = refreshed.Required
	s.ScheduledInstallationDay = refreshed.ScheduledInstallationDay
	s.ScheduledInstallationTime = refreshed.ScheduledInstallationTime

	return nil
}

// Save applies the current Automatic Updates settings.
// https://learn.microsoft.com/en-us/windows/win32/api/wuapi/nf-wuapi-iautomaticupdatessettings-save
func (s *IAutomaticUpdatesSettings) Save() error {
	_, err := oleutil.CallMethod(s.disp, "Save")
	return err
}

// PutNotificationLevel sets the notification level for Automatic Updates.
// https://learn.microsoft.com/en-us/windows/win32/api/wuapi/nf-wuapi-iautomaticupdatessettings-put_notificationlevel
func (s *IAutomaticUpdatesSettings) PutNotificationLevel(level int32) error {
	_, err := oleutil.PutProperty(s.disp, "NotificationLevel", level)
	if err != nil {
		return err
	}
	s.NotificationLevel = level
	return nil
}

// PutScheduledInstallationDay sets the scheduled installation day.
// Note: Not supported on Windows 8 and later.
// https://learn.microsoft.com/en-us/windows/win32/api/wuapi/nf-wuapi-iautomaticupdatessettings-put_scheduledinstallationday
func (s *IAutomaticUpdatesSettings) PutScheduledInstallationDay(day int32) error {
	_, err := oleutil.PutProperty(s.disp, "ScheduledInstallationDay", day)
	if err != nil {
		return err
	}
	s.ScheduledInstallationDay = day
	return nil
}

// PutScheduledInstallationTime sets the scheduled installation time (hour of day, 0-23).
// Note: Not supported on Windows 8 and later.
// https://learn.microsoft.com/en-us/windows/win32/api/wuapi/nf-wuapi-iautomaticupdatessettings-put_scheduledinstallationtime
func (s *IAutomaticUpdatesSettings) PutScheduledInstallationTime(hour int32) error {
	_, err := oleutil.PutProperty(s.disp, "ScheduledInstallationTime", hour)
	if err != nil {
		return err
	}
	s.ScheduledInstallationTime = hour
	return nil
}
