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

// OperationResultCode defines the possible results of a download, install, uninstall, or verification operation on an update.
// https://docs.microsoft.com/en-us/windows/win32/api/wuapi/ne-wuapi-operationresultcode
const (
	OperationResultCodeOrcNotStarted int32 = iota
	OperationResultCodeOrcInProgress
	OperationResultCodeOrcSucceeded
	OperationResultCodeOrcSucceededWithErrors
	OperationResultCodeOrcFailed
	OperationResultCodeOrcAborted
)

// DeploymentAction defines the action for which an update is eligible.
// https://docs.microsoft.com/en-us/windows/win32/api/wuapi/ne-wuapi-deploymentaction
const (
	DeploymentActionDaNone int32 = iota
	DeploymentActionDaDetection
	DeploymentActionDaInstallation
	DeploymentActionDaUninstallation
	DeploymentActionDaOptionalInstallation
)

// DownloadPriority defines the priority of a download.
// https://docs.microsoft.com/en-us/windows/win32/api/wuapi/ne-wuapi-downloadpriority
const (
	DownloadPriorityDpLow    int32 = 1
	DownloadPriorityDpNormal int32 = 2
	DownloadPriorityDpHigh   int32 = 3
)

// InstallationImpact defines the impact of installing an update.
// https://docs.microsoft.com/en-us/windows/win32/api/wuapi/ne-wuapi-installationimpact
const (
	InstallationImpactIiNormal int32 = iota
	InstallationImpactIiMinor
	InstallationImpactIiRequiresExclusiveHandling
)

// InstallationRebootBehavior defines the restart behavior of an update.
// https://docs.microsoft.com/en-us/windows/win32/api/wuapi/ne-wuapi-installationrebootbehavior
const (
	InstallationRebootBehaviorIrbNeverReboots int32 = iota
	InstallationRebootBehaviorIrbAlwaysRequiresReboot
	InstallationRebootBehaviorIrbCanRequestReboot
)

// UpdateOperation defines the operation for which an update is being installed or uninstalled.
// https://docs.microsoft.com/en-us/windows/win32/api/wuapi/ne-wuapi-updateoperation
const (
	UpdateOperationUoInstallation   int32 = 1
	UpdateOperationUoUninstallation int32 = 2
)

// UpdateExceptionContext defines the context in which an IUpdateException object can be provided.
// https://docs.microsoft.com/en-us/windows/win32/api/wuapi/ne-wuapi-updateexceptioncontext
const (
	UpdateExceptionContextUecGeneral int32 = iota + 1
	UpdateExceptionContextUecWindowsDriver
	UpdateExceptionContextUecWindowsInstaller
	UpdateExceptionContextUecSearchIncomplete
)

// ServerSelection defines the update server that is used for a search or download operation.
// https://learn.microsoft.com/en-us/windows/win32/api/wuapi/ne-wuapi-serverselection
const (
	ServerSelectionSsDefault       int32 = iota // Use the default server.
	ServerSelectionSsManagedServer              // Use the managed server (WSUS).
	ServerSelectionSsWindowsUpdate              // Use Windows Update.
	ServerSelectionSsOthers                     // Use a non-Microsoft server.
)

// AutomaticUpdatesNotificationLevel defines the notification level for automatic updates.
// https://learn.microsoft.com/en-us/windows/win32/api/wuapi/ne-wuapi-automaticupdatesnotificationlevel
const (
	AutomaticUpdatesNotificationLevelAunlNotConfigured            int32 = iota // Not configured.
	AutomaticUpdatesNotificationLevelAunlDisabled                              // Disabled.
	AutomaticUpdatesNotificationLevelAunlNotifyBeforeDownload                  // Notify before download.
	AutomaticUpdatesNotificationLevelAunlNotifyBeforeInstallation              // Notify before installation.
	AutomaticUpdatesNotificationLevelAunlScheduledInstallation                 // Scheduled installation.
)

// AutomaticUpdatesScheduledInstallationDay defines the days of the week for scheduled automatic updates.
// https://learn.microsoft.com/en-us/windows/win32/api/wuapi/ne-wuapi-automaticupdatesscheduledinstallationday
const (
	AutomaticUpdatesScheduledInstallationDayAuisdEveryDay       int32 = iota // Every day.
	AutomaticUpdatesScheduledInstallationDayAuisdEverySunday                 // Every Sunday.
	AutomaticUpdatesScheduledInstallationDayAuisdEveryMonday                 // Every Monday.
	AutomaticUpdatesScheduledInstallationDayAuisdEveryTuesday                // Every Tuesday.
	AutomaticUpdatesScheduledInstallationDayAuisdEveryWednesday              // Every Wednesday.
	AutomaticUpdatesScheduledInstallationDayAuisdEveryThursday               // Every Thursday.
	AutomaticUpdatesScheduledInstallationDayAuisdEveryFriday                 // Every Friday.
	AutomaticUpdatesScheduledInstallationDayAuisdEverySaturday               // Every Saturday.
)

// DownloadPhase defines the phase of the download.
// https://learn.microsoft.com/en-us/windows/win32/api/wuapi/ne-wuapi-downloadphase
const (
	DownloadPhaseInitializing int32 = iota + 1
	DownloadPhaseDownloading
	DownloadPhaseVerifying
)

// AutoDownload defines auto download behavior for IUpdate5.
// https://learn.microsoft.com/en-us/windows/win32/api/wuapi/ne-wuapi-autodownloadmode
const (
	AutoDownloadModeForbidAutoDownload int32 = iota
	AutoDownloadModeAllowAutoDownload
)

// AutoSelection defines auto selection behavior for IUpdate5.
// https://learn.microsoft.com/en-us/windows/win32/api/wuapi/ne-wuapi-autoselectionmode
const (
	AutoSelectionModeLetWindowsUpdateDecide int32 = iota
	AutoSelectionModeAutoSelectIfDownloaded
	AutoSelectionModeNeverAutoSelect
	AutoSelectionModeAlwaysAutoSelect
)

// UpdateServiceRegistrationState defines the state of a service registration.
// https://learn.microsoft.com/en-us/windows/win32/api/wuapi/ne-wuapi-updateserviceregistrationstate
const (
	UpdateServiceRegistrationStateNotRegistered int32 = iota + 1
	UpdateServiceRegistrationStateRegistrationPending
	UpdateServiceRegistrationStateRegistered
)

// AddServiceFlag defines flags for AddService2.
// https://learn.microsoft.com/en-us/windows/win32/api/wuapi/ne-wuapi-addserviceflag
const (
	AddServiceFlagAsfAllowPendingRegistration int32 = 1
	AddServiceFlagAsfAllowOnlineRegistration  int32 = 2
	AddServiceFlagAsfRegisterServiceWithAU    int32 = 4
)

// UpdateType defines the type of an update.
// https://learn.microsoft.com/en-us/windows/win32/api/wuapi/ne-wuapi-updatetype
const (
	UpdateTypeSoftware int32 = iota + 1
	UpdateTypeDriver
)
