import QtQuick
import QtQuick.Controls.Basic
import QtQuick.Layouts
import com.redflag.desktop 1.0

ApplicationWindow {
    id: app

    property string page: "overview"
    property var history: parse(machine.historyJson, [])
    property var system: parse(machine.systemJson, {
    })
    property var processes: parse(machine.processesJson, [])
    property var connections: parse(machine.connectionsJson, [])
    property var containers: parse(machine.containersJson, [])
    property var services: parse(machine.servicesJson, [])
    property var updates: parse(machine.updatesJson, [])
    property var security: parse(machine.securityJson, {
    })
    property var events: parse(machine.eventsJson, [])
    property var processDetail: parse(machine.processDetailJson, {
    })
    property var softwareSnapshot: parse(machine.softwareJson, {
    })
    property var software: softwareSnapshot.packages || []
    property var softwareDetail: parse(machine.softwareDetailJson, {
    })
    property var selectedUpdate: ({
    })
    property string pendingProcessQuery: ""
    property var approval: parse(machine.approvalJson, {
    })
    // The capabilities that turn "a process" into "a process that can rewrite the
    // machine". Tinted so a wall of chips still reads at a glance.
    readonly property var sharpCapabilities: ["CAP_SYS_ADMIN", "CAP_SYS_MODULE", "CAP_SYS_RAWIO", "CAP_SYS_PTRACE", "CAP_SYS_BOOT", "CAP_BPF", "CAP_NET_ADMIN", "CAP_NET_RAW", "CAP_DAC_READ_SEARCH", "CAP_SETUID", "CAP_SETGID"]

    function parse(raw, fallback) {
        try {
            return JSON.parse(raw || "");
        } catch (_) {
            return fallback;
        }
    }

    function bytes(value) {
        const n = Number(value || 0);
        if (n <= 0)
            return "0 B";

        const units = ["B", "KiB", "MiB", "GiB", "TiB"];
        const i = Math.min(units.length - 1, Math.floor(Math.log(n) / Math.log(1024)));
        const amount = n / Math.pow(1024, i);
        return (amount >= 10 || i === 0 ? amount.toFixed(0) : amount.toFixed(1)) + " " + units[i];
    }

    function percent(value) {
        return Number(value || 0).toFixed(0) + "%";
    }

    function endpoint(address, port) {
        return (address || "*") + ":" + Number(port || 0);
    }

    // The Agent reports the kernel's full container ID; the container inventory
    // carries the 12-character short form. The join is a prefix, never equality.
    function containerFor(id) {
        if (!id)
            return null;

        for (let i = 0; i < containers.length; ++i) if (containers[i].container_id && String(id).indexOf(String(containers[i].container_id)) === 0) {
            return containers[i];
        }
        return null;
    }

    function ownerLabel(p) {
        if (!p)
            return "";

        if (p.container_id) {
            const c = containerFor(p.container_id);
            return c && c.name ? c.name : (p.container_runtime || "container") + " " + String(p.container_id).substring(0, 12);
        }
        return p.unit || "";
    }

    function ownerColor(p) {
        return p && p.container_id ? Theme.cyan : Theme.blue;
    }

    // Services report the unit name with .service stripped; processes carry it whole.
    function unitProcessCount(name) {
        if (!name)
            return 0;

        const unit = name + ".service";
        let n = 0;
        for (let i = 0; i < processes.length; ++i) if (processes[i].unit === unit) {
            n++;
        }
        return n;
    }

    // CapEff arrives as a 64-bit hex mask. Split it into two 32-bit words rather
    // than trusting a single parseInt, which loses precision above bit 52.
    function holdsSharpCapability(mask) {
        if (!mask)
            return false;

        const hex = String(mask);
        const low = parseInt(hex.slice(-8) || "0", 16) || 0;
        const high = parseInt(hex.slice(0, -8) || "0", 16) || 0;
        // low word: DAC_READ_SEARCH 2, SETGID 6, SETUID 7, NET_ADMIN 12,
        // SYS_MODULE 16, SYS_RAWIO 17, SYS_PTRACE 19, SYS_ADMIN 21, SYS_BOOT 22
        const lowMask = (1 << 2) | (1 << 6) | (1 << 7) | (1 << 12) | (1 << 16) | (1 << 17) | (1 << 19) | (1 << 21) | (1 << 22);
        // high word: PERFMON 38, BPF 39 → bits 6 and 7 of the high word
        const highMask = (1 << 6) | (1 << 7);
        return (low & lowMask) !== 0 || (high & highMask) !== 0;
    }

    function sharpProcessCount() {
        let n = 0;
        for (let i = 0; i < processes.length; ++i) if (holdsSharpCapability(processes[i].capabilities_effective)) {
            n++;
        }
        return n;
    }

    function capabilityColor(name) {
        return sharpCapabilities.indexOf(name) >= 0 ? Theme.warn : Theme.dim;
    }

    // The four stages the fleet dashboard shows, driven here by what the Agent
    // actually returned rather than by a hardcoded chip.
    function stageState(index) {
        const a = approval;
        const result = a.receipt || a.policy || {
        };
        if (index === 0)
            return "done";

        if (a.error)
            return index === 1 ? "failed" : "idle";

        if (machine.approvalRunning)
            return index === 1 ? "running" : "idle";

        if (a.request_id === undefined)
            return "idle";

        if (index === 1)
            return "done";

        if (index === 2)
            return (result.authorization_id || result.token_id) ? "done" : "failed";

        if (index === 3)
            return result.executed ? "done" : "failed";

        return "idle";
    }

    function stageColor(state) {
        if (state === "done")
            return Theme.good;

        if (state === "running")
            return Theme.warn;

        if (state === "failed")
            return Theme.bad;

        return Theme.faint;
    }

    function locallyApprovable(u) {
        return !!u && (u.package_type === "dnf" || u.package_type === "apt" || u.package_type === "pacman");
    }

    function approvalBlockReason(u) {
        if (!u || !u.package_name)
            return "";

        if (machine.enrollment === "fleet enrolled")
            return "This machine is fleet enrolled — authority belongs to the RedFlag Server, and the Agent refuses a local mint.";

        if (!locallyApprovable(u))
            return "Local approval covers DNF, APT, and pacman. " + String(u.package_type || "this ecosystem").toUpperCase() + " still executes through the fleet signed-command path.";

        return "";
    }

    function approvalReasonRequired(u) {
        return !!u && u.package_type === "pacman";
    }

    function approvalCanOverride() {
        return String(approval.error || "").indexOf("osv_status=") >= 0;
    }

    function approvalResult() {
        return approval.receipt || approval.policy || {
        };
    }

    function verifiedActionCount() {
        const result = approvalResult();
        if (result.verified_actions !== undefined)
            return result.verified_actions;

        if (result.verified_artifacts !== undefined)
            return result.verified_artifacts;

        return "—";
    }

    function processCountLabel(n) {
        return n ? n + (n === 1 ? " process" : " processes") : "";
    }

    function containerProcessCount(id) {
        if (!id)
            return 0;

        let n = 0;
        for (let i = 0; i < processes.length; ++i) if (processes[i].container_id && String(processes[i].container_id).indexOf(String(id)) === 0) {
            n++;
        }
        return n;
    }

    function historyCeiling(first, second) {
        let maximum = 1;
        for (let i = 0; i < history.length; ++i) maximum = Math.max(maximum, Number(history[i][first] || 0) + Number(history[i][second] || 0))
        return maximum;
    }

    function navBadge(key) {
        if (key === "processes")
            return String(machine.processCount);

        if (key === "containers")
            return String(machine.containerCount);

        if (key === "services")
            return String(machine.serviceCount);

        if (key === "software")
            return String(machine.softwareCount);

        if (key === "updates")
            return String(machine.updateCount);

        if (key === "security" && machine.criticalCount > 0)
            return String(machine.criticalCount);

        return "";
    }

    function pageComponent() {
        switch (page) {
        case "performance":
            return performancePage;
        case "processes":
            return processesPage;
        case "network":
            return networkPage;
        case "storage":
            return storagePage;
        case "containers":
            return containersPage;
        case "services":
            return servicesPage;
        case "software":
            return softwarePage;
        case "updates":
            return updatesPage;
        case "security":
            return securityPage;
        case "history":
            return historyPage;
        default:
            return overviewPage;
        }
    }

    title: "RedFlag"
    width: 1320
    height: 860
    minimumWidth: 960
    minimumHeight: 640
    visible: true
    color: Theme.window
    Component.onCompleted: {
        machine.refreshTelemetry();
        machine.refreshOverview();
        machine.refreshProcesses();
        machine.refreshConnections();
        machine.refreshSoftware();
    }
    // Containers and Services join against the process list; entering either page
    // refreshes it once rather than polling a full snapshot every few seconds.
    onPageChanged: {
        if (page === "processes" || page === "containers" || page === "services" || page === "security")
            machine.refreshProcesses();

        if (page === "network")
            machine.refreshConnections();

        if (page === "software")
            machine.refreshSoftware();

    }

    Machine {
        id: machine

        onProcessDetailUpdated: processDialog.open()
        onSoftwareDetailUpdated: softwareDialog.open()
        onApprovalFinished: {
            machine.refreshOverview();
            machine.refreshSoftware();
        }
    }

    Timer {
        interval: 1000
        repeat: true
        running: true
        onTriggered: machine.refreshTelemetry()
    }

    Timer {
        interval: 15000
        repeat: true
        running: true
        onTriggered: machine.refreshOverview()
    }

    Timer {
        interval: 3000
        repeat: true
        running: true
        onTriggered: {
            if (app.page === "processes")
                machine.refreshProcesses();

            if (app.page === "network")
                machine.refreshConnections();

        }
    }

    RowLayout {
        anchors.fill: parent
        spacing: 0

        Rectangle {
            Layout.preferredWidth: Theme.railWidth
            Layout.fillHeight: true
            color: Theme.rail
            border.width: 0

            ColumnLayout {
                anchors.fill: parent
                anchors.margins: 12
                spacing: 4

                RowLayout {
                    Layout.fillWidth: true
                    Layout.preferredHeight: 54
                    Layout.leftMargin: 4
                    spacing: 10

                    Image {
                        width: 30
                        height: 30
                        source: "qrc:/qt/qml/com/redflag/desktop/icons/icon.png"
                        fillMode: Image.PreserveAspectFit
                        smooth: true
                    }

                    Column {
                        Text {
                            text: "RedFlag"
                            color: Theme.text
                            font.pixelSize: 19
                            font.weight: 750
                        }

                        Text {
                            text: "THIS MACHINE"
                            color: Theme.faint
                            font.pixelSize: 9
                            font.letterSpacing: 1.4
                        }

                    }

                    Item {
                        Layout.fillWidth: true
                    }

                }

                Text {
                    text: "OBSERVE"
                    color: Theme.faint
                    font.pixelSize: 9
                    font.letterSpacing: 1.2
                    Layout.leftMargin: 8
                    Layout.topMargin: 8
                    Layout.bottomMargin: 2
                }

                Repeater {
                    model: [{
                        "key": "overview",
                        "label": "Overview"
                    }, {
                        "key": "performance",
                        "label": "Performance"
                    }, {
                        "key": "processes",
                        "label": "Processes"
                    }, {
                        "key": "network",
                        "label": "Network"
                    }, {
                        "key": "storage",
                        "label": "Storage"
                    }]

                    NavItem {
                        required property var modelData

                        Layout.fillWidth: true
                        label: modelData.label
                        badge: app.navBadge(modelData.key)
                        selected: app.page === modelData.key
                        onClicked: app.page = modelData.key
                    }

                }

                Text {
                    text: "OPERATE"
                    color: Theme.faint
                    font.pixelSize: 9
                    font.letterSpacing: 1.2
                    Layout.leftMargin: 8
                    Layout.topMargin: 10
                    Layout.bottomMargin: 2
                }

                Repeater {
                    model: [{
                        "key": "containers",
                        "label": "Containers"
                    }, {
                        "key": "services",
                        "label": "Services"
                    }, {
                        "key": "software",
                        "label": "Software"
                    }, {
                        "key": "updates",
                        "label": "Updates"
                    }]

                    NavItem {
                        required property var modelData

                        Layout.fillWidth: true
                        label: modelData.label
                        badge: app.navBadge(modelData.key)
                        selected: app.page === modelData.key
                        onClicked: app.page = modelData.key
                    }

                }

                Text {
                    text: "TRUST"
                    color: Theme.faint
                    font.pixelSize: 9
                    font.letterSpacing: 1.2
                    Layout.leftMargin: 8
                    Layout.topMargin: 10
                    Layout.bottomMargin: 2
                }

                Repeater {
                    model: [{
                        "key": "security",
                        "label": "Security"
                    }, {
                        "key": "history",
                        "label": "History"
                    }]

                    NavItem {
                        required property var modelData

                        Layout.fillWidth: true
                        label: modelData.label
                        badge: app.navBadge(modelData.key)
                        selected: app.page === modelData.key
                        onClicked: app.page = modelData.key
                    }

                }

                Item {
                    Layout.fillHeight: true
                }

                Rectangle {
                    Layout.fillWidth: true
                    height: 1
                    color: Theme.divider
                }

                RowLayout {
                    Layout.fillWidth: true
                    Layout.preferredHeight: 46
                    Layout.leftMargin: 5

                    Rectangle {
                        width: 8
                        height: 8
                        radius: 4
                        color: machine.connected ? Theme.good : Theme.bad
                    }

                    Column {
                        Text {
                            text: machine.connected ? "Agent connected" : "Agent unavailable"
                            color: Theme.dim
                            font.pixelSize: 11
                        }

                        Text {
                            text: machine.enrollment + (machine.agentVersion.length ? " · v" + machine.agentVersion : "")
                            color: Theme.faint
                            font.pixelSize: 9
                        }

                    }

                }

            }

        }

        Rectangle {
            Layout.preferredWidth: 1
            Layout.fillHeight: true
            color: Theme.divider
        }

        ColumnLayout {
            Layout.fillWidth: true
            Layout.fillHeight: true
            spacing: 0

            Rectangle {
                Layout.fillWidth: true
                Layout.preferredHeight: Theme.headerHeight
                color: Theme.window
                border.width: 0

                RowLayout {
                    anchors.fill: parent
                    anchors.leftMargin: 24
                    anchors.rightMargin: 22
                    spacing: 14

                    Rectangle {
                        width: 10
                        height: 10
                        radius: 5
                        color: !machine.connected ? Theme.faint : machine.healthState === "healthy" ? Theme.good : Theme.warn
                    }

                    ColumnLayout {
                        spacing: 1

                        Text {
                            text: machine.hostname
                            color: Theme.text
                            font.pixelSize: Theme.fontHeading
                            font.weight: 650
                        }

                        Text {
                            text: machine.machineSubtitle
                            color: Theme.dim
                            font.pixelSize: Theme.fontMeta
                        }

                    }

                    Rectangle {
                        width: healthLabel.implicitWidth + 18
                        height: 25
                        radius: 13
                        color: Theme.tint(machine.healthState === "healthy" ? Theme.good : machine.healthState === "degraded" ? Theme.warn : Theme.faint, 0.14)

                        Text {
                            id: healthLabel

                            anchors.centerIn: parent
                            text: machine.healthState.toUpperCase()
                            color: machine.healthState === "healthy" ? Theme.good : machine.healthState === "degraded" ? Theme.warn : Theme.faint
                            font.pixelSize: 9
                            font.weight: 700
                            font.letterSpacing: 1
                        }

                    }

                    Item {
                        Layout.fillWidth: true
                    }

                    Text {
                        text: machine.uptime.length ? "UP " + machine.uptime.toUpperCase() : ""
                        color: Theme.faint
                        font.pixelSize: 9
                        font.family: Theme.mono
                    }

                    Rectangle {
                        width: 32
                        height: 32
                        radius: 16
                        color: refreshMouse.containsMouse ? Theme.hover : "transparent"

                        Text {
                            anchors.centerIn: parent
                            text: "↻"
                            color: Theme.dim
                            font.pixelSize: 20
                        }

                        MouseArea {
                            id: refreshMouse

                            anchors.fill: parent
                            hoverEnabled: true
                            onClicked: {
                                machine.refreshTelemetry();
                                machine.refreshOverview();
                            }
                        }

                    }

                    Rectangle {
                        width: 32
                        height: 32
                        radius: 16
                        color: themeMouse.containsMouse ? Theme.hover : "transparent"

                        Text {
                            anchors.centerIn: parent
                            text: Theme.dark ? "◐" : "◑"
                            color: Theme.dim
                            font.pixelSize: 15
                        }

                        MouseArea {
                            id: themeMouse

                            anchors.fill: parent
                            hoverEnabled: true
                            onClicked: Theme.dark = !Theme.dark
                        }

                    }

                }

                Rectangle {
                    anchors.bottom: parent.bottom
                    width: parent.width
                    height: 1
                    color: Theme.divider
                }

            }

            Rectangle {
                visible: machine.connected && machine.agentGap.length > 0
                Layout.fillWidth: true
                Layout.preferredHeight: visible ? gapText.implicitHeight + 20 : 0
                color: Theme.tint(Theme.warn, 0.12)

                Text {
                    id: gapText

                    anchors.fill: parent
                    anchors.margins: 10
                    text: machine.agentGap
                    color: Theme.warn
                    font.pixelSize: 11
                    wrapMode: Text.Wrap
                }

            }

            Rectangle {
                visible: !machine.connected && machine.connectionError.length > 0
                Layout.fillWidth: true
                Layout.preferredHeight: visible ? errorText.implicitHeight + 20 : 0
                color: Theme.redDim

                Text {
                    id: errorText

                    anchors.fill: parent
                    anchors.margins: 10
                    text: machine.connectionError
                    color: Theme.bad
                    font.pixelSize: 11
                    font.family: Theme.mono
                    wrapMode: Text.Wrap
                }

            }

            Loader {
                Layout.fillWidth: true
                Layout.fillHeight: true
                sourceComponent: app.pageComponent()
            }

        }

    }

    Component {
        id: overviewPage

        ScrollView {
            clip: true
            contentWidth: availableWidth

            ColumnLayout {
                width: parent.width
                spacing: 16
                anchors.margins: 22

                Text {
                    text: "This machine"
                    color: Theme.text
                    font.pixelSize: Theme.fontTitle
                    font.weight: 700
                    Layout.topMargin: 20
                }

                Text {
                    text: "Operational condition, live load, and the systems asking for attention."
                    color: Theme.dim
                    font.pixelSize: Theme.fontBody
                }

                GridLayout {
                    Layout.fillWidth: true
                    columns: 4
                    columnSpacing: 12
                    rowSpacing: 12

                    MetricCard {
                        Layout.fillWidth: true
                        label: "CPU"
                        value: app.percent(machine.cpuUsage)
                        detail: "load " + machine.load1.toFixed(2)
                        accent: Theme.pressure(machine.cpuUsage)
                    }

                    MetricCard {
                        Layout.fillWidth: true
                        label: "Memory"
                        value: app.percent(machine.memoryPercent)
                        detail: app.bytes(machine.memoryUsed) + " / " + app.bytes(machine.memoryTotal)
                        accent: Theme.pressure(machine.memoryPercent)
                    }

                    MetricCard {
                        Layout.fillWidth: true
                        label: "Network"
                        value: "↓ " + app.bytes(machine.networkReceive) + "/s"
                        detail: "↑ " + app.bytes(machine.networkTransmit) + "/s"
                        accent: Theme.cyan
                    }

                    MetricCard {
                        Layout.fillWidth: true
                        label: "Temperature"
                        value: machine.temperature > 0 ? machine.temperature.toFixed(0) + " °C" : "Unavailable"
                        detail: machine.temperature > 0 ? "hottest reported sensor" : "no sensor evidence"
                        accent: machine.temperature >= 90 ? Theme.bad : Theme.purple
                    }

                    MetricCard {
                        Layout.fillWidth: true
                        label: "Updates"
                        value: String(machine.updateCount)
                        detail: machine.criticalCount + " critical"
                        accent: machine.criticalCount > 0 ? Theme.bad : machine.updateCount > 0 ? Theme.warn : Theme.good
                    }

                    MetricCard {
                        Layout.fillWidth: true
                        label: "Containers"
                        value: machine.containerRunning + " / " + machine.containerCount
                        detail: machine.containerUnhealthy + " unhealthy"
                        accent: machine.containerUnhealthy > 0 ? Theme.bad : Theme.good
                    }

                    MetricCard {
                        Layout.fillWidth: true
                        label: "Services"
                        value: machine.serviceRunning + " / " + machine.serviceCount
                        detail: machine.serviceFailed + " failed"
                        accent: machine.serviceFailed > 0 ? Theme.bad : Theme.good
                    }

                    MetricCard {
                        Layout.fillWidth: true
                        label: "Processes"
                        value: String(machine.processCount)
                        detail: machine.agentStatus + " · " + machine.enrollment
                        accent: Theme.blue
                    }

                }

                RowLayout {
                    Layout.fillWidth: true
                    Layout.preferredHeight: 250
                    spacing: 12

                    Rectangle {
                        Layout.fillWidth: true
                        Layout.fillHeight: true
                        color: Theme.surface
                        radius: Theme.radius
                        border.width: 1
                        border.color: Theme.divider

                        Text {
                            text: "CPU — LAST " + app.history.length + " SECONDS"
                            color: Theme.faint
                            font.pixelSize: 10
                            font.letterSpacing: 1
                            anchors.left: parent.left
                            anchors.leftMargin: 16
                            anchors.top: parent.top
                            anchors.topMargin: 14
                        }

                        Text {
                            text: app.percent(machine.cpuUsage)
                            color: Theme.text
                            font.pixelSize: 28
                            font.weight: 650
                            anchors.right: parent.right
                            anchors.rightMargin: 16
                            anchors.top: parent.top
                            anchors.topMargin: 10
                        }

                        LineGraph {
                            anchors.fill: parent
                            anchors.margins: 16
                            anchors.topMargin: 54
                            values: app.history.map((p) => {
                                return p.cpu_percent;
                            })
                            ceiling: 100
                            lineColor: Theme.red
                        }

                    }

                    Rectangle {
                        Layout.preferredWidth: 360
                        Layout.fillHeight: true
                        color: Theme.surface
                        radius: Theme.radius
                        border.width: 1
                        border.color: Theme.divider

                        Text {
                            text: "RECENT"
                            color: Theme.faint
                            font.pixelSize: 10
                            font.letterSpacing: 1
                            anchors.left: parent.left
                            anchors.leftMargin: 16
                            anchors.top: parent.top
                            anchors.topMargin: 14
                        }

                        ListView {
                            anchors.fill: parent
                            anchors.topMargin: 40
                            clip: true
                            model: app.events.slice(0, 6)

                            Text {
                                visible: app.events.length === 0
                                anchors.centerIn: parent
                                text: "No local events awaiting sync."
                                color: Theme.faint
                                font.pixelSize: 11
                            }

                            delegate: Rectangle {
                                required property var modelData

                                width: ListView.view.width
                                height: 35
                                color: "transparent"

                                Rectangle {
                                    width: 5
                                    height: 5
                                    radius: 3
                                    color: modelData.severity === "critical" || modelData.severity === "error" ? Theme.bad : modelData.severity === "warning" ? Theme.warn : Theme.faint
                                    anchors.left: parent.left
                                    anchors.leftMargin: 16
                                    anchors.top: parent.top
                                    anchors.topMargin: 9
                                }

                                Text {
                                    text: modelData.message || modelData.event_type || "event"
                                    color: Theme.dim
                                    font.pixelSize: 11
                                    anchors.left: parent.left
                                    anchors.leftMargin: 30
                                    anchors.right: parent.right
                                    anchors.rightMargin: 12
                                    anchors.top: parent.top
                                    anchors.topMargin: 4
                                    elide: Text.ElideRight
                                }

                                Text {
                                    text: modelData.component || "agent"
                                    color: Theme.faint
                                    font.pixelSize: 9
                                    anchors.left: parent.left
                                    anchors.leftMargin: 30
                                    anchors.bottom: parent.bottom
                                    anchors.bottomMargin: 3
                                }

                            }

                        }

                    }

                }

                Item {
                    Layout.preferredHeight: 22
                }

            }

        }

    }

    Component {
        id: performancePage

        ScrollView {
            clip: true
            contentWidth: availableWidth

            ColumnLayout {
                width: parent.width
                spacing: 14
                anchors.margins: 22

                Text {
                    text: "Performance"
                    color: Theme.text
                    font.pixelSize: Theme.fontTitle
                    font.weight: 700
                    Layout.topMargin: 20
                }

                Text {
                    text: "Agent-owned one-second telemetry. History is bounded to five minutes in memory."
                    color: Theme.dim
                    font.pixelSize: Theme.fontBody
                }

                GridLayout {
                    Layout.fillWidth: true
                    columns: 4
                    columnSpacing: 12

                    MetricCard {
                        Layout.fillWidth: true
                        label: "CPU"
                        value: app.percent(machine.cpuUsage)
                        detail: "load " + machine.load1.toFixed(2) + " · " + machine.load5.toFixed(2) + " · " + machine.load15.toFixed(2)
                        accent: Theme.red
                    }

                    MetricCard {
                        Layout.fillWidth: true
                        label: "Memory"
                        value: app.percent(machine.memoryPercent)
                        detail: app.bytes(machine.memoryUsed) + " used"
                        accent: Theme.blue
                    }

                    MetricCard {
                        Layout.fillWidth: true
                        label: "Swap"
                        value: app.percent(machine.swapPercent)
                        detail: "memory pressure reserve"
                        accent: Theme.purple
                    }

                    MetricCard {
                        Layout.fillWidth: true
                        label: "Disk I/O"
                        value: app.bytes(machine.diskRead + machine.diskWrite) + "/s"
                        detail: "↓ " + app.bytes(machine.diskRead) + "  ↑ " + app.bytes(machine.diskWrite)
                        accent: Theme.warn
                    }

                }

                Rectangle {
                    Layout.fillWidth: true
                    Layout.preferredHeight: 280
                    color: Theme.surface
                    radius: Theme.radius
                    border.width: 1
                    border.color: Theme.divider

                    Text {
                        text: "CPU UTILIZATION"
                        color: Theme.faint
                        font.pixelSize: 10
                        font.letterSpacing: 1
                        anchors.left: parent.left
                        anchors.leftMargin: 16
                        anchors.top: parent.top
                        anchors.topMargin: 14
                    }

                    Text {
                        text: app.percent(machine.cpuUsage)
                        color: Theme.text
                        font.pixelSize: 30
                        font.weight: 650
                        anchors.right: parent.right
                        anchors.rightMargin: 16
                        anchors.top: parent.top
                        anchors.topMargin: 9
                    }

                    LineGraph {
                        anchors.fill: parent
                        anchors.margins: 18
                        anchors.topMargin: 58
                        values: app.history.map((p) => {
                            return p.cpu_percent;
                        })
                        ceiling: 100
                        lineColor: Theme.red
                    }

                }

                RowLayout {
                    Layout.fillWidth: true
                    Layout.preferredHeight: 220
                    spacing: 12

                    Rectangle {
                        Layout.fillWidth: true
                        Layout.fillHeight: true
                        color: Theme.surface
                        radius: Theme.radius
                        border.width: 1
                        border.color: Theme.divider

                        Text {
                            text: "MEMORY"
                            color: Theme.faint
                            font.pixelSize: 10
                            font.letterSpacing: 1
                            anchors.left: parent.left
                            anchors.leftMargin: 16
                            anchors.top: parent.top
                            anchors.topMargin: 14
                        }

                        LineGraph {
                            anchors.fill: parent
                            anchors.margins: 16
                            anchors.topMargin: 44
                            values: app.history.map((p) => {
                                return p.memory_percent;
                            })
                            ceiling: 100
                            lineColor: Theme.blue
                        }

                    }

                    Rectangle {
                        Layout.fillWidth: true
                        Layout.fillHeight: true
                        color: Theme.surface
                        radius: Theme.radius
                        border.width: 1
                        border.color: Theme.divider

                        Text {
                            text: "NETWORK THROUGHPUT"
                            color: Theme.faint
                            font.pixelSize: 10
                            font.letterSpacing: 1
                            anchors.left: parent.left
                            anchors.leftMargin: 16
                            anchors.top: parent.top
                            anchors.topMargin: 14
                        }

                        LineGraph {
                            anchors.fill: parent
                            anchors.margins: 16
                            anchors.topMargin: 44
                            values: app.history.map((p) => {
                                return Number(p.network_receive_per_second) + Number(p.network_transmit_per_second);
                            })
                            ceiling: app.historyCeiling("network_receive_per_second", "network_transmit_per_second")
                            lineColor: Theme.cyan
                        }

                    }

                }

                Item {
                    Layout.preferredHeight: 22
                }

            }

        }

    }

    Component {
        id: processesPage

        Item {
            id: processRoot

            property string query: search.text.toLowerCase()
            property var rows: app.processes.filter((p) => {
                return !query.length || String(p.name).toLowerCase().includes(query) || String(p.cmdline).toLowerCase().includes(query) || String(p.user).toLowerCase().includes(query) || String(p.pid).includes(query) || String(app.ownerLabel(p)).toLowerCase().includes(query) || String(p.unit || "").toLowerCase().includes(query);
            })

            Component.onCompleted: {
                if (app.pendingProcessQuery.length) {
                    search.text = app.pendingProcessQuery;
                    app.pendingProcessQuery = "";
                }
            }

            ColumnLayout {
                anchors.fill: parent
                anchors.margins: 22
                spacing: 12

                RowLayout {
                    Layout.fillWidth: true

                    Column {
                        Text {
                            text: "Processes"
                            color: Theme.text
                            font.pixelSize: Theme.fontTitle
                            font.weight: 700
                        }

                        Text {
                            text: rows.length + " visible · select a process for sockets, files, namespaces, and identity"
                            color: Theme.dim
                            font.pixelSize: Theme.fontBody
                        }

                    }

                    Item {
                        Layout.fillWidth: true
                    }

                    TextField {
                        id: search

                        Layout.preferredWidth: 300
                        placeholderText: "Find process, PID, command, owner"
                        color: Theme.text
                        placeholderTextColor: Theme.faint

                        background: Rectangle {
                            color: Theme.surface
                            border.width: 1
                            border.color: search.activeFocus ? Theme.red : Theme.divider
                            radius: Theme.radiusSm
                        }

                    }

                }

                Rectangle {
                    Layout.fillWidth: true
                    Layout.fillHeight: true
                    color: Theme.surface
                    radius: Theme.radius
                    border.width: 1
                    border.color: Theme.divider

                    ColumnLayout {
                        anchors.fill: parent
                        spacing: 0

                        Rectangle {
                            Layout.fillWidth: true
                            Layout.preferredHeight: 34
                            color: Theme.raised

                            RowLayout {
                                anchors.fill: parent
                                anchors.leftMargin: 14
                                anchors.rightMargin: 14

                                Text {
                                    text: "PROCESS"
                                    color: Theme.faint
                                    font.pixelSize: 9
                                    Layout.fillWidth: true
                                }

                                Text {
                                    text: "PID"
                                    color: Theme.faint
                                    font.pixelSize: 9
                                    Layout.preferredWidth: 70
                                }

                                Text {
                                    text: "OWNER"
                                    color: Theme.faint
                                    font.pixelSize: 9
                                    Layout.preferredWidth: 120
                                }

                                Text {
                                    text: "CPU"
                                    color: Theme.faint
                                    font.pixelSize: 9
                                    Layout.preferredWidth: 70
                                }

                                Text {
                                    text: "MEMORY"
                                    color: Theme.faint
                                    font.pixelSize: 9
                                    Layout.preferredWidth: 80
                                }

                            }

                        }

                        ListView {
                            Layout.fillWidth: true
                            Layout.fillHeight: true
                            clip: true
                            model: processRoot.rows

                            delegate: Rectangle {
                                required property var modelData

                                width: ListView.view.width
                                height: 47
                                color: rowMouse.containsMouse ? Theme.hover : "transparent"

                                Rectangle {
                                    anchors.bottom: parent.bottom
                                    width: parent.width
                                    height: 1
                                    color: Theme.divider
                                }

                                RowLayout {
                                    anchors.fill: parent
                                    anchors.leftMargin: 14
                                    anchors.rightMargin: 14

                                    Column {
                                        Layout.fillWidth: true

                                        Text {
                                            text: modelData.name || "unknown"
                                            color: Theme.text
                                            font.pixelSize: 12
                                            font.family: Theme.mono
                                        }

                                        Row {
                                            id: subRow

                                            width: parent.width
                                            spacing: 6

                                            Text {
                                                id: ownerTag

                                                visible: text.length > 0
                                                text: app.ownerLabel(modelData)
                                                color: app.ownerColor(modelData)
                                                font.pixelSize: 9
                                                font.family: Theme.mono
                                            }

                                            Text {
                                                id: ownerDot

                                                visible: ownerTag.visible
                                                text: "·"
                                                color: Theme.faint
                                                font.pixelSize: 9
                                            }

                                            Text {
                                                width: subRow.width - (ownerTag.visible ? ownerTag.width + ownerDot.width + subRow.spacing * 2 : 0)
                                                text: modelData.cmdline || modelData.path || ""
                                                color: Theme.faint
                                                font.pixelSize: 9
                                                elide: Text.ElideRight
                                            }

                                        }

                                    }

                                    Text {
                                        text: String(modelData.pid)
                                        color: Theme.dim
                                        font.pixelSize: 11
                                        font.family: Theme.mono
                                        Layout.preferredWidth: 70
                                    }

                                    Text {
                                        text: modelData.user || String(modelData.uid)
                                        color: Theme.dim
                                        font.pixelSize: 11
                                        Layout.preferredWidth: 120
                                        elide: Text.ElideRight
                                    }

                                    Text {
                                        text: Number(modelData.cpu_percent || 0).toFixed(1) + "%"
                                        color: Theme.pressure(Number(modelData.cpu_percent || 0))
                                        font.pixelSize: 11
                                        Layout.preferredWidth: 70
                                    }

                                    Text {
                                        text: app.bytes(modelData.rss_bytes)
                                        color: Theme.dim
                                        font.pixelSize: 11
                                        Layout.preferredWidth: 80
                                    }

                                }

                                MouseArea {
                                    id: rowMouse

                                    anchors.fill: parent
                                    hoverEnabled: true
                                    onClicked: machine.loadProcess(Number(modelData.pid))
                                }

                            }

                        }

                    }

                }

            }

        }

    }

    Component {
        id: networkPage

        Item {
            id: networkRoot

            property string query: networkSearch.text.toLowerCase()
            property var rows: app.connections.filter((c) => {
                return !query.length || String(c.process).toLowerCase().includes(query) || String(c.local_addr).includes(query) || String(c.remote_addr).includes(query) || String(c.local_port).includes(query);
            })

            ColumnLayout {
                anchors.fill: parent
                anchors.margins: 22
                spacing: 12

                RowLayout {
                    Layout.fillWidth: true

                    Column {
                        Text {
                            text: "Network"
                            color: Theme.text
                            font.pixelSize: Theme.fontTitle
                            font.weight: 700
                        }

                        Text {
                            text: "Interfaces, throughput, listeners, and live socket ownership."
                            color: Theme.dim
                            font.pixelSize: Theme.fontBody
                        }

                    }

                    Item {
                        Layout.fillWidth: true
                    }

                    TextField {
                        id: networkSearch

                        Layout.preferredWidth: 280
                        placeholderText: "Find process, address, port"
                        color: Theme.text
                        placeholderTextColor: Theme.faint

                        background: Rectangle {
                            color: Theme.surface
                            border.width: 1
                            border.color: networkSearch.activeFocus ? Theme.red : Theme.divider
                            radius: Theme.radiusSm
                        }

                    }

                }

                RowLayout {
                    Layout.fillWidth: true

                    MetricCard {
                        Layout.fillWidth: true
                        label: "Receive"
                        value: app.bytes(machine.networkReceive) + "/s"
                        detail: "all non-loopback interfaces"
                        accent: Theme.cyan
                    }

                    MetricCard {
                        Layout.fillWidth: true
                        label: "Transmit"
                        value: app.bytes(machine.networkTransmit) + "/s"
                        detail: "all non-loopback interfaces"
                        accent: Theme.purple
                    }

                    MetricCard {
                        Layout.fillWidth: true
                        label: "Sockets"
                        value: String(rows.length)
                        detail: "TCP and UDP · process-correlated"
                        accent: Theme.blue
                    }

                }

                Rectangle {
                    Layout.fillWidth: true
                    Layout.fillHeight: true
                    color: Theme.surface
                    radius: Theme.radius
                    border.width: 1
                    border.color: Theme.divider

                    ColumnLayout {
                        anchors.fill: parent
                        spacing: 0

                        Rectangle {
                            Layout.fillWidth: true
                            Layout.preferredHeight: 34
                            color: Theme.raised

                            RowLayout {
                                anchors.fill: parent
                                anchors.leftMargin: 14
                                anchors.rightMargin: 14

                                Text {
                                    text: "PROCESS"
                                    color: Theme.faint
                                    font.pixelSize: 9
                                    Layout.preferredWidth: 180
                                }

                                Text {
                                    text: "PROTOCOL"
                                    color: Theme.faint
                                    font.pixelSize: 9
                                    Layout.preferredWidth: 80
                                }

                                Text {
                                    text: "LOCAL"
                                    color: Theme.faint
                                    font.pixelSize: 9
                                    Layout.fillWidth: true
                                }

                                Text {
                                    text: "REMOTE"
                                    color: Theme.faint
                                    font.pixelSize: 9
                                    Layout.fillWidth: true
                                }

                                Text {
                                    text: "STATE"
                                    color: Theme.faint
                                    font.pixelSize: 9
                                    Layout.preferredWidth: 100
                                }

                            }

                        }

                        ListView {
                            Layout.fillWidth: true
                            Layout.fillHeight: true
                            clip: true
                            model: networkRoot.rows

                            delegate: Rectangle {
                                required property var modelData

                                width: ListView.view.width
                                height: 38
                                color: "transparent"

                                Rectangle {
                                    anchors.bottom: parent.bottom
                                    width: parent.width
                                    height: 1
                                    color: Theme.divider
                                }

                                RowLayout {
                                    anchors.fill: parent
                                    anchors.leftMargin: 14
                                    anchors.rightMargin: 14

                                    Text {
                                        text: (modelData.process || "unknown") + (modelData.pid ? " · " + modelData.pid : "")
                                        color: Theme.text
                                        font.pixelSize: 11
                                        font.family: Theme.mono
                                        Layout.preferredWidth: 180
                                        elide: Text.ElideRight
                                    }

                                    Text {
                                        text: modelData.protocol + " " + modelData.family
                                        color: Theme.dim
                                        font.pixelSize: 10
                                        Layout.preferredWidth: 80
                                    }

                                    Text {
                                        text: app.endpoint(modelData.local_addr, modelData.local_port)
                                        color: Theme.dim
                                        font.pixelSize: 10
                                        font.family: Theme.mono
                                        Layout.fillWidth: true
                                        elide: Text.ElideRight
                                    }

                                    Text {
                                        text: modelData.remote_port ? app.endpoint(modelData.remote_addr, modelData.remote_port) : "—"
                                        color: Theme.dim
                                        font.pixelSize: 10
                                        font.family: Theme.mono
                                        Layout.fillWidth: true
                                        elide: Text.ElideRight
                                    }

                                    Text {
                                        text: modelData.state
                                        color: modelData.state === "LISTEN" ? Theme.warn : modelData.state === "ESTABLISHED" ? Theme.good : Theme.faint
                                        font.pixelSize: 10
                                        Layout.preferredWidth: 100
                                    }

                                }

                                MouseArea {
                                    anchors.fill: parent
                                    hoverEnabled: true
                                    onEntered: parent.color = Theme.hover
                                    onExited: parent.color = "transparent"
                                    onClicked: {
                                        if (modelData.pid) {
                                            machine.loadProcess(Number(modelData.pid));
                                        }
                                    }
                                }

                            }

                        }

                    }

                }

            }

        }

    }

    Component {
        id: storagePage

        ScrollView {
            clip: true
            contentWidth: availableWidth

            ColumnLayout {
                width: parent.width
                anchors.margins: 22
                spacing: 14

                Text {
                    text: "Storage"
                    color: Theme.text
                    font.pixelSize: Theme.fontTitle
                    font.weight: 700
                    Layout.topMargin: 20
                }

                Text {
                    text: "Filesystems and physical-device throughput observed by the Agent."
                    color: Theme.dim
                    font.pixelSize: Theme.fontBody
                }

                RowLayout {
                    Layout.fillWidth: true

                    MetricCard {
                        Layout.fillWidth: true
                        label: "Read"
                        value: app.bytes(machine.diskRead) + "/s"
                        detail: "physical block devices"
                        accent: Theme.blue
                    }

                    MetricCard {
                        Layout.fillWidth: true
                        label: "Write"
                        value: app.bytes(machine.diskWrite) + "/s"
                        detail: "physical block devices"
                        accent: Theme.warn
                    }

                }

                Repeater {
                    model: app.system.disk_info || []

                    Rectangle {
                        required property var modelData

                        Layout.fillWidth: true
                        Layout.preferredHeight: 88
                        color: Theme.surface
                        radius: Theme.radius
                        border.width: 1
                        border.color: Theme.divider

                        RowLayout {
                            anchors.fill: parent
                            anchors.margins: 15

                            Column {
                                Layout.fillWidth: true

                                Text {
                                    text: modelData.mountpoint + (modelData.is_root ? " · ROOT" : "")
                                    color: Theme.text
                                    font.pixelSize: 13
                                    font.family: Theme.mono
                                    font.weight: 600
                                }

                                Text {
                                    text: modelData.filesystem + " · " + app.bytes(modelData.used) + " used · " + app.bytes(modelData.available) + " available"
                                    color: Theme.dim
                                    font.pixelSize: 11
                                }

                            }

                            Text {
                                text: app.percent(modelData.used_percent)
                                color: Theme.pressure(modelData.used_percent)
                                font.pixelSize: 22
                                font.weight: 650
                            }

                        }

                        Rectangle {
                            anchors.left: parent.left
                            anchors.right: parent.right
                            anchors.bottom: parent.bottom
                            anchors.margins: 15
                            height: 4
                            radius: 2
                            color: Theme.divider

                            Rectangle {
                                width: parent.width * Math.min(1, Number(modelData.used_percent) / 100)
                                height: parent.height
                                radius: 2
                                color: Theme.pressure(modelData.used_percent)
                            }

                        }

                    }

                }

                Item {
                    Layout.preferredHeight: 22
                }

            }

        }

    }

    Component {
        id: containersPage

        Item {
            ColumnLayout {
                anchors.fill: parent
                anchors.margins: 22
                spacing: 12

                Text {
                    text: "Containers"
                    color: Theme.text
                    font.pixelSize: Theme.fontTitle
                    font.weight: 700
                }

                Text {
                    text: "Docker containers and Compose/stack identity from the Agent's existing container model."
                    color: Theme.dim
                    font.pixelSize: Theme.fontBody
                }

                RowLayout {
                    Layout.fillWidth: true

                    MetricCard {
                        Layout.fillWidth: true
                        label: "Containers"
                        value: machine.containerRunning + " / " + machine.containerCount
                        detail: "running / total"
                        accent: Theme.blue
                    }

                    MetricCard {
                        Layout.fillWidth: true
                        label: "Health"
                        value: machine.containerUnhealthy ? machine.containerUnhealthy + " unhealthy" : "Healthy"
                        detail: "reported Docker health checks"
                        accent: machine.containerUnhealthy ? Theme.bad : Theme.good
                    }

                }

                Rectangle {
                    Layout.fillWidth: true
                    Layout.fillHeight: true
                    color: Theme.surface
                    radius: Theme.radius
                    border.width: 1
                    border.color: Theme.divider

                    ListView {
                        anchors.fill: parent
                        clip: true
                        model: app.containers

                        Text {
                            visible: app.containers.length === 0
                            anchors.centerIn: parent
                            text: "Docker unavailable or no containers are present."
                            color: Theme.faint
                            font.pixelSize: 12
                        }

                        delegate: Rectangle {
                            required property var modelData

                            width: ListView.view.width
                            height: 58
                            color: "transparent"

                            Rectangle {
                                anchors.bottom: parent.bottom
                                width: parent.width
                                height: 1
                                color: Theme.divider
                            }

                            RowLayout {
                                anchors.fill: parent
                                anchors.margins: 13

                                Rectangle {
                                    width: 8
                                    height: 8
                                    radius: 4
                                    color: modelData.health === "unhealthy" ? Theme.bad : modelData.state === "running" ? Theme.good : Theme.faint
                                }

                                Column {
                                    Layout.fillWidth: true

                                    Text {
                                        text: modelData.name || modelData.container_id
                                        color: Theme.text
                                        font.pixelSize: 12
                                        font.family: Theme.mono
                                    }

                                    Text {
                                        text: modelData.image + (modelData.stack_name ? " · " + modelData.stack_name : "")
                                        color: Theme.dim
                                        font.pixelSize: 10
                                    }

                                }

                                Text {
                                    text: app.processCountLabel(app.containerProcessCount(modelData.container_id))
                                    color: Theme.cyan
                                    font.pixelSize: 10
                                    font.family: Theme.mono
                                    Layout.preferredWidth: 100
                                }

                                Text {
                                    text: modelData.ports || "no published ports"
                                    color: Theme.faint
                                    font.pixelSize: 10
                                    font.family: Theme.mono
                                    Layout.preferredWidth: 260
                                    elide: Text.ElideRight
                                }

                                Text {
                                    text: (modelData.health || modelData.state || "unknown").toUpperCase()
                                    color: modelData.health === "unhealthy" ? Theme.bad : modelData.state === "running" ? Theme.good : Theme.faint
                                    font.pixelSize: 9
                                    font.weight: 700
                                    Layout.preferredWidth: 90
                                }

                            }

                            MouseArea {
                                anchors.fill: parent
                                hoverEnabled: true
                                onEntered: parent.color = Theme.hover
                                onExited: parent.color = "transparent"
                                onClicked: {
                                    app.pendingProcessQuery = String(modelData.name || modelData.container_id || "");
                                    app.page = "processes";
                                }
                            }

                        }

                    }

                }

            }

        }

    }

    Component {
        id: servicesPage

        Item {
            id: servicesRoot

            property string query: serviceSearch.text.toLowerCase()
            property var rows: app.services.filter((s) => {
                return !query.length || String(s.name).toLowerCase().includes(query) || String(s.description).toLowerCase().includes(query);
            })

            ColumnLayout {
                anchors.fill: parent
                anchors.margins: 22
                spacing: 12

                RowLayout {
                    Layout.fillWidth: true

                    Column {
                        Text {
                            text: "Services"
                            color: Theme.text
                            font.pixelSize: Theme.fontTitle
                            font.weight: 700
                        }

                        Text {
                            text: "Cross-platform service domain, currently backed by systemd observation on Linux."
                            color: Theme.dim
                            font.pixelSize: Theme.fontBody
                        }

                    }

                    Item {
                        Layout.fillWidth: true
                    }

                    TextField {
                        id: serviceSearch

                        Layout.preferredWidth: 280
                        placeholderText: "Find service"
                        color: Theme.text
                        placeholderTextColor: Theme.faint

                        background: Rectangle {
                            color: Theme.surface
                            border.width: 1
                            border.color: serviceSearch.activeFocus ? Theme.red : Theme.divider
                            radius: Theme.radiusSm
                        }

                    }

                }

                RowLayout {
                    Layout.fillWidth: true

                    MetricCard {
                        Layout.fillWidth: true
                        label: "Running"
                        value: String(machine.serviceRunning)
                        detail: "active services"
                        accent: Theme.good
                    }

                    MetricCard {
                        Layout.fillWidth: true
                        label: "Failed"
                        value: String(machine.serviceFailed)
                        detail: "requires operator attention"
                        accent: machine.serviceFailed ? Theme.bad : Theme.good
                    }

                }

                Rectangle {
                    Layout.fillWidth: true
                    Layout.fillHeight: true
                    color: Theme.surface
                    radius: Theme.radius
                    border.width: 1
                    border.color: Theme.divider

                    ListView {
                        anchors.fill: parent
                        clip: true
                        model: servicesRoot.rows

                        delegate: Rectangle {
                            required property var modelData

                            width: ListView.view.width
                            height: 48
                            color: "transparent"

                            Rectangle {
                                anchors.bottom: parent.bottom
                                width: parent.width
                                height: 1
                                color: Theme.divider
                            }

                            RowLayout {
                                anchors.fill: parent
                                anchors.margins: 13

                                Rectangle {
                                    width: 7
                                    height: 7
                                    radius: 4
                                    color: modelData.active_state === "failed" ? Theme.bad : modelData.active_state === "active" ? Theme.good : Theme.faint
                                }

                                Column {
                                    Layout.fillWidth: true

                                    Text {
                                        text: modelData.name
                                        color: Theme.text
                                        font.pixelSize: 12
                                        font.family: Theme.mono
                                    }

                                    Text {
                                        text: modelData.description || ""
                                        color: Theme.faint
                                        font.pixelSize: 10
                                        elide: Text.ElideRight
                                    }

                                }

                                Text {
                                    text: app.processCountLabel(app.unitProcessCount(modelData.name))
                                    color: Theme.blue
                                    font.pixelSize: 10
                                    font.family: Theme.mono
                                    Layout.preferredWidth: 100
                                }

                                Text {
                                    text: (modelData.active_state + " · " + modelData.sub_state).toUpperCase()
                                    color: modelData.active_state === "failed" ? Theme.bad : modelData.active_state === "active" ? Theme.good : Theme.faint
                                    font.pixelSize: 9
                                    font.weight: 700
                                    Layout.preferredWidth: 130
                                }

                            }

                            MouseArea {
                                anchors.fill: parent
                                hoverEnabled: true
                                onEntered: parent.color = Theme.hover
                                onExited: parent.color = "transparent"
                                onClicked: {
                                    app.pendingProcessQuery = String(modelData.name || "") + ".service";
                                    app.page = "processes";
                                }
                            }

                        }

                    }

                }

            }

        }

    }

    Component {
        id: softwarePage

        Item {
            id: softwareRoot

            property string query: softwareSearch.text.toLowerCase()
            property var rows: app.software.filter((p) => {
                return !query.length || String(p.name).toLowerCase().includes(query) || String(p.description).toLowerCase().includes(query) || String(p.package_type).toLowerCase().includes(query);
            })

            ColumnLayout {
                anchors.fill: parent
                anchors.margins: 22
                spacing: 12

                RowLayout {
                    Layout.fillWidth: true

                    Column {
                        Text {
                            text: "Software"
                            color: Theme.text
                            font.pixelSize: Theme.fontTitle
                            font.weight: 700
                        }

                        Text {
                            text: "Everything the machine's package managers say is installed. Select one to follow its ownership and dependency edges."
                            color: Theme.dim
                            font.pixelSize: Theme.fontBody
                        }

                    }

                    Item {
                        Layout.fillWidth: true
                    }

                    TextField {
                        id: softwareSearch

                        Layout.preferredWidth: 320
                        placeholderText: "Find installed software"
                        color: Theme.text
                        placeholderTextColor: Theme.faint

                        background: Rectangle {
                            color: Theme.surface
                            border.width: 1
                            border.color: softwareSearch.activeFocus ? Theme.red : Theme.divider
                            radius: Theme.radiusSm
                        }

                    }

                    Button {
                        text: machine.softwareLoading ? "Reading…" : "Refresh"
                        enabled: !machine.softwareLoading
                        onClicked: machine.refreshSoftware()
                    }

                }

                RowLayout {
                    Layout.fillWidth: true

                    MetricCard {
                        Layout.fillWidth: true
                        label: "Installed"
                        value: String(machine.softwareCount)
                        detail: "known local packages"
                        accent: Theme.blue
                    }

                    MetricCard {
                        Layout.fillWidth: true
                        label: "Explicit"
                        value: String(machine.softwareExplicitCount)
                        detail: "chosen by an operator"
                        accent: Theme.cyan
                    }

                    MetricCard {
                        Layout.fillWidth: true
                        label: "Dependencies"
                        value: String(machine.softwareDependencyCount)
                        detail: "pulled into closures"
                        accent: Theme.purple
                    }

                    MetricCard {
                        Layout.fillWidth: true
                        label: "Foreign"
                        value: String(machine.softwareForeignCount)
                        detail: "outside known repositories"
                        accent: machine.softwareForeignCount > 0 ? Theme.warn : Theme.good
                    }

                }

                Rectangle {
                    Layout.fillWidth: true
                    Layout.fillHeight: true
                    color: Theme.surface
                    radius: Theme.radius
                    border.width: 1
                    border.color: Theme.divider

                    ListView {
                        anchors.fill: parent
                        clip: true
                        model: softwareRoot.rows

                        Text {
                            visible: app.software.length === 0 && !machine.softwareLoading
                            anchors.centerIn: parent
                            text: "The Agent reported no installed package inventory."
                            color: Theme.faint
                            font.pixelSize: 12
                        }

                        delegate: Rectangle {
                            required property var modelData

                            width: ListView.view.width
                            height: 58
                            color: packageMouse.containsMouse ? Theme.hover : "transparent"

                            Rectangle {
                                anchors.bottom: parent.bottom
                                width: parent.width
                                height: 1
                                color: Theme.divider
                            }

                            RowLayout {
                                anchors.fill: parent
                                anchors.margins: 13

                                Column {
                                    Layout.fillWidth: true

                                    Text {
                                        text: modelData.name || modelData.identity
                                        color: Theme.text
                                        font.pixelSize: 12
                                        font.family: Theme.mono
                                        font.weight: 600
                                    }

                                    Text {
                                        text: modelData.description || "No description reported"
                                        color: Theme.dim
                                        font.pixelSize: 10
                                        elide: Text.ElideRight
                                        width: parent.width
                                    }

                                }

                                Text {
                                    text: modelData.version || "unknown"
                                    color: Theme.text
                                    font.pixelSize: 10
                                    font.family: Theme.mono
                                    Layout.preferredWidth: 180
                                    elide: Text.ElideRight
                                }

                                Text {
                                    text: String(modelData.install_reason || "unknown").toUpperCase()
                                    color: modelData.install_reason === "explicit" ? Theme.cyan : Theme.faint
                                    font.pixelSize: 9
                                    font.weight: 700
                                    Layout.preferredWidth: 100
                                }

                                Text {
                                    text: String(modelData.package_type || "unknown").toUpperCase()
                                    color: Theme.faint
                                    font.pixelSize: 9
                                    font.weight: 700
                                    Layout.preferredWidth: 72
                                }

                                Text {
                                    text: modelData.origin === "foreign" ? "FOREIGN" : ""
                                    color: Theme.warn
                                    font.pixelSize: 9
                                    font.weight: 700
                                    Layout.preferredWidth: 64
                                }

                            }

                            MouseArea {
                                id: packageMouse

                                anchors.fill: parent
                                hoverEnabled: true
                                onClicked: machine.loadSoftware(modelData.package_type || "", modelData.identity || modelData.name || "")
                            }

                        }

                    }

                }

            }

        }

    }

    Component {
        id: updatesPage

        Item {
            ColumnLayout {
                anchors.fill: parent
                anchors.margins: 22
                spacing: 12

                RowLayout {
                    Layout.fillWidth: true

                    Column {
                        Text {
                            text: "Updates"
                            color: Theme.text
                            font.pixelSize: Theme.fontTitle
                            font.weight: 700
                        }

                        Text {
                            text: "Available changes, vulnerability evidence, and the RedFlag mutation path."
                            color: Theme.dim
                            font.pixelSize: Theme.fontBody
                        }

                    }

                    Item {
                        Layout.fillWidth: true
                    }

                    Button {
                        text: machine.overviewLoading ? "Scanning…" : "Scan now"
                        enabled: !machine.overviewLoading
                        onClicked: machine.triggerScan()

                        background: Rectangle {
                            color: parent.pressed ? Qt.darker(Theme.red, 1.15) : Theme.red
                            radius: Theme.radiusSm
                        }

                        contentItem: Text {
                            text: parent.text
                            color: "white"
                            horizontalAlignment: Text.AlignHCenter
                            verticalAlignment: Text.AlignVCenter
                            font.pixelSize: 11
                            font.weight: 650
                        }

                    }

                }

                Rectangle {
                    visible: machine.operationMessage.length > 0
                    Layout.fillWidth: true
                    Layout.preferredHeight: 36
                    color: Theme.redDim
                    radius: Theme.radiusSm

                    Text {
                        anchors.centerIn: parent
                        text: machine.operationMessage
                        color: Theme.dim
                        font.pixelSize: 11
                    }

                }

                Rectangle {
                    Layout.fillWidth: true
                    Layout.fillHeight: true
                    color: Theme.surface
                    radius: Theme.radius
                    border.width: 1
                    border.color: Theme.divider

                    ListView {
                        anchors.fill: parent
                        clip: true
                        model: app.updates

                        Text {
                            visible: app.updates.length === 0
                            anchors.centerIn: parent
                            text: "No pending updates."
                            color: Theme.good
                            font.pixelSize: 12
                        }

                        delegate: Rectangle {
                            required property var modelData

                            width: ListView.view.width
                            height: 68
                            color: "transparent"

                            Rectangle {
                                anchors.bottom: parent.bottom
                                width: parent.width
                                height: 1
                                color: Theme.divider
                            }

                            RowLayout {
                                anchors.fill: parent
                                anchors.margins: 14

                                Rectangle {
                                    width: 8
                                    height: 8
                                    radius: 4
                                    color: modelData.severity === "critical" ? Theme.bad : modelData.severity === "high" || modelData.severity === "important" ? Theme.warn : Theme.faint
                                }

                                Column {
                                    Layout.fillWidth: true

                                    Text {
                                        text: modelData.package_name
                                        color: Theme.text
                                        font.pixelSize: 13
                                        font.family: Theme.mono
                                        font.weight: 600
                                    }

                                    Text {
                                        text: (modelData.current_version || "?") + "  →  " + (modelData.available_version || "?") + (modelData.cve_list && modelData.cve_list.length ? " · " + modelData.cve_list.length + " advisories" : "")
                                        color: Theme.dim
                                        font.pixelSize: 10
                                        font.family: Theme.mono
                                    }

                                }

                                Text {
                                    text: String(modelData.package_type || "unknown").toUpperCase()
                                    color: Theme.faint
                                    font.pixelSize: 9
                                    font.weight: 700
                                    Layout.preferredWidth: 90
                                }

                                Column {
                                    Layout.preferredWidth: 150

                                    Text {
                                        text: "DISCOVERED"
                                        color: Theme.warn
                                        font.pixelSize: 9
                                        font.weight: 700
                                        anchors.right: parent.right
                                    }

                                    Text {
                                        text: app.locallyApprovable(modelData) && machine.enrollment !== "fleet enrolled" ? "authorize here" : "awaiting authorization"
                                        color: Theme.faint
                                        font.pixelSize: 9
                                        anchors.right: parent.right
                                    }

                                }

                            }

                            MouseArea {
                                anchors.fill: parent
                                hoverEnabled: true
                                onEntered: parent.color = Theme.hover
                                onExited: parent.color = "transparent"
                                onClicked: {
                                    app.selectedUpdate = modelData;
                                    machine.approvalJson = "{}";
                                    updateDialog.open();
                                }
                            }

                        }

                    }

                }

                Text {
                    text: "Package execution remains Agent → signed envelope → helper. This glass does not own a package-manager escape hatch."
                    color: Theme.faint
                    font.pixelSize: 10
                    Layout.alignment: Qt.AlignHCenter
                }

            }

        }

    }

    Component {
        id: securityPage

        ScrollView {
            clip: true
            contentWidth: availableWidth

            ColumnLayout {
                width: parent.width
                anchors.margins: 22
                spacing: 14

                Text {
                    text: "Security & supply chain"
                    color: Theme.text
                    font.pixelSize: Theme.fontTitle
                    font.weight: 700
                    Layout.topMargin: 20
                }

                Text {
                    text: "Evidence and enforcement state. Unknown stays unknown; no invented green checkmarks."
                    color: Theme.dim
                    font.pixelSize: Theme.fontBody
                }

                GridLayout {
                    Layout.fillWidth: true
                    columns: 2
                    columnSpacing: 12
                    rowSpacing: 12

                    Repeater {
                        model: [{
                            "label": "Command signing",
                            "value": app.security.command_signing_enabled ? "Enabled" : "Disabled",
                            "detail": app.security.command_enforcement || "unknown",
                            "ok": app.security.command_signing_enabled && app.security.command_enforcement === "strict"
                        }, {
                            "label": "TLS verification",
                            "value": app.security.tls_verification ? "Enforced" : "Bypassed",
                            "detail": "Agent to fleet server",
                            "ok": app.security.tls_verification
                        }, {
                            "label": "Security logging",
                            "value": app.security.security_logging ? "Enabled" : "Disabled",
                            "detail": "local security event channel",
                            "ok": app.security.security_logging
                        }, {
                            "label": "Kernel enforcement",
                            "value": app.security.kernel_enforcement ? "Configured" : "Not configured",
                            "detail": app.security.kernel_fail_closed ? "fail closed" : "fail open / unknown",
                            "ok": app.security.kernel_enforcement && app.security.kernel_fail_closed
                        }, {
                            "label": "Critical updates",
                            "value": String(machine.criticalCount),
                            "detail": "reported package findings",
                            "ok": machine.criticalCount === 0
                        }, {
                            "label": "Agent mode",
                            "value": app.security.degraded_mode ? "Degraded" : "Normal",
                            "detail": machine.enrollment,
                            "ok": !app.security.degraded_mode
                        }, {
                            "label": "Capability tokens",
                            "value": String((app.security.capabilities || {
                            }).pending_count || 0) + " pending",
                            "detail": ((app.security.capabilities || {
                            }).last_failed_count || 0) + " failed · " + ((app.security.capabilities || {
                            }).last_processed_count || 0) + " processed",
                            "ok": ((app.security.capabilities || {
                            }).last_failed_count || 0) === 0
                        }, {
                            "label": "Privileged processes",
                            "value": app.processes.length ? String(app.sharpProcessCount()) : "unscanned",
                            "detail": app.processes.length ? "hold a capability that can rewrite this machine" : "open Processes to scan",
                            "ok": app.processes.length > 0 && app.sharpProcessCount() === 0
                        }]

                        Rectangle {
                            required property var modelData

                            Layout.fillWidth: true
                            Layout.preferredHeight: 94
                            color: Theme.surface
                            radius: Theme.radius
                            border.width: 1
                            border.color: Theme.divider

                            Rectangle {
                                width: 4
                                height: parent.height - 26
                                radius: 2
                                color: modelData.ok ? Theme.good : Theme.warn
                                anchors.left: parent.left
                                anchors.leftMargin: 14
                                anchors.verticalCenter: parent.verticalCenter
                            }

                            Column {
                                anchors.left: parent.left
                                anchors.leftMargin: 31
                                anchors.verticalCenter: parent.verticalCenter

                                Text {
                                    text: modelData.label.toUpperCase()
                                    color: Theme.faint
                                    font.pixelSize: 9
                                    font.letterSpacing: 1
                                }

                                Text {
                                    text: modelData.value
                                    color: modelData.ok ? Theme.good : Theme.warn
                                    font.pixelSize: 19
                                    font.weight: 650
                                }

                                Text {
                                    text: modelData.detail
                                    color: Theme.dim
                                    font.pixelSize: 10
                                }

                            }

                        }

                    }

                }

                Rectangle {
                    Layout.fillWidth: true
                    Layout.preferredHeight: 100
                    color: Theme.redDim
                    radius: Theme.radius
                    border.width: 1
                    border.color: Theme.tint(Theme.red, 0.45)

                    Column {
                        anchors.fill: parent
                        anchors.margins: 15
                        spacing: 5

                        Text {
                            text: "AUTHORITY BOUNDARY"
                            color: Theme.red
                            font.pixelSize: 10
                            font.weight: 750
                            font.letterSpacing: 1
                        }

                        Text {
                            width: parent.width
                            text: "QML observes RedFlag state and expresses operator intent. The Agent resolves policy and authorization. The privileged helper verifies a bounded mutation envelope before any platform backend runs."
                            color: Theme.text
                            font.pixelSize: 12
                            wrapMode: Text.Wrap
                        }

                        Text {
                            text: "Override uses the same machinery and must leave actor, reason, target, bypass, and aftermath in history."
                            color: Theme.dim
                            font.pixelSize: 10
                        }

                    }

                }

                Item {
                    Layout.preferredHeight: 22
                }

            }

        }

    }

    Component {
        id: historyPage

        Item {
            ColumnLayout {
                anchors.fill: parent
                anchors.margins: 22
                spacing: 12

                Text {
                    text: "History"
                    color: Theme.text
                    font.pixelSize: Theme.fontTitle
                    font.weight: 700
                }

                Text {
                    text: "Local operational events awaiting fleet synchronization. Mutation receipts join this same timeline."
                    color: Theme.dim
                    font.pixelSize: Theme.fontBody
                }

                Rectangle {
                    Layout.fillWidth: true
                    Layout.fillHeight: true
                    color: Theme.surface
                    radius: Theme.radius
                    border.width: 1
                    border.color: Theme.divider

                    ListView {
                        anchors.fill: parent
                        clip: true
                        model: app.events

                        Text {
                            visible: app.events.length === 0
                            anchors.centerIn: parent
                            text: "No local events are waiting to synchronize."
                            color: Theme.faint
                            font.pixelSize: 12
                        }

                        delegate: Rectangle {
                            required property var modelData

                            width: ListView.view.width
                            height: 64
                            color: "transparent"

                            Rectangle {
                                anchors.bottom: parent.bottom
                                width: parent.width
                                height: 1
                                color: Theme.divider
                            }

                            RowLayout {
                                anchors.fill: parent
                                anchors.margins: 14

                                Rectangle {
                                    width: 8
                                    height: 8
                                    radius: 4
                                    color: modelData.severity === "critical" || modelData.severity === "error" ? Theme.bad : modelData.severity === "warning" ? Theme.warn : Theme.faint
                                }

                                Column {
                                    Layout.fillWidth: true

                                    Text {
                                        text: modelData.message || modelData.event_type
                                        color: Theme.text
                                        font.pixelSize: 12
                                        elide: Text.ElideRight
                                        width: parent.width
                                    }

                                    Text {
                                        text: (modelData.component || "agent") + " · " + (modelData.event_subtype || "event")
                                        color: Theme.dim
                                        font.pixelSize: 10
                                    }

                                }

                                Text {
                                    text: modelData.created_at ? new Date(modelData.created_at).toLocaleString() : ""
                                    color: Theme.faint
                                    font.pixelSize: 9
                                    Layout.preferredWidth: 180
                                    horizontalAlignment: Text.AlignRight
                                }

                            }

                        }

                    }

                }

            }

        }

    }

    Dialog {
        id: processDialog

        width: Math.min(760, app.width - 80)
        height: Math.min(650, app.height - 80)
        anchors.centerIn: parent
        modal: true
        title: app.processDetail.name ? app.processDetail.name + " · PID " + app.processDetail.pid : "Process"

        background: Rectangle {
            color: Theme.surface
            radius: Theme.radius
            border.width: 1
            border.color: Theme.divider
        }

        header: Rectangle {
            width: parent.width
            height: 54
            color: Theme.raised

            Text {
                anchors.left: parent.left
                anchors.leftMargin: 16
                anchors.verticalCenter: parent.verticalCenter
                text: processDialog.title
                color: Theme.text
                font.pixelSize: 16
                font.weight: 650
            }

        }

        contentItem: ScrollView {
            clip: true

            ColumnLayout {
                width: processDialog.availableWidth
                spacing: 8

                Text {
                    text: app.processDetail.error || app.processDetail.path || "Path unavailable"
                    color: app.processDetail.error ? Theme.bad : Theme.dim
                    font.pixelSize: 11
                    font.family: Theme.mono
                    wrapMode: Text.Wrap
                    Layout.fillWidth: true
                }

                Text {
                    text: app.processDetail.cmdline || ""
                    color: Theme.text
                    font.pixelSize: 11
                    font.family: Theme.mono
                    wrapMode: Text.Wrap
                    Layout.fillWidth: true
                }

                Text {
                    visible: text.length > 0
                    text: app.processDetail.cgroup || ""
                    color: Theme.faint
                    font.pixelSize: 10
                    font.family: Theme.mono
                    wrapMode: Text.Wrap
                    Layout.fillWidth: true
                }

                GridLayout {
                    columns: 4
                    Layout.fillWidth: true
                    columnSpacing: 12
                    rowSpacing: 8

                    Repeater {
                        model: [{
                            "l": "Owner",
                            "v": app.processDetail.user || app.processDetail.uid || "—"
                        }, {
                            "l": "State",
                            "v": app.processDetail.state || "—"
                        }, {
                            "l": "CPU",
                            "v": Number(app.processDetail.cpu_percent || 0).toFixed(1) + "%"
                        }, {
                            "l": "Memory",
                            "v": app.bytes(app.processDetail.rss_bytes)
                        }, {
                            "l": "Threads",
                            "v": app.processDetail.threads || 0
                        }, {
                            "l": "Parent PID",
                            "v": app.processDetail.parent_pid || 0
                        }, {
                            "l": "Read",
                            "v": app.bytes(app.processDetail.disk_bytes_read)
                        }, {
                            "l": "Written",
                            "v": app.bytes(app.processDetail.disk_bytes_written)
                        }, {
                            "l": "Belongs to",
                            "v": app.ownerLabel(app.processDetail) || "unattributed"
                        }, {
                            "l": "Package",
                            "v": app.processDetail.package_name ? app.processDetail.package_name + " · " + String(app.processDetail.package_manager || "").toUpperCase() : "unattributed"
                        }, {
                            "l": "Elevation",
                            "v": app.processDetail.elevation_status || "normal"
                        }]

                        Rectangle {
                            required property var modelData

                            Layout.fillWidth: true
                            Layout.preferredHeight: 62
                            color: Theme.raised
                            radius: Theme.radiusSm

                            Column {
                                anchors.centerIn: parent

                                Text {
                                    anchors.horizontalCenter: parent.horizontalCenter
                                    text: modelData.l.toUpperCase()
                                    color: Theme.faint
                                    font.pixelSize: 8
                                }

                                Text {
                                    anchors.horizontalCenter: parent.horizontalCenter
                                    text: modelData.v
                                    color: Theme.text
                                    font.pixelSize: 12
                                    font.family: Theme.mono
                                }

                            }

                        }

                    }

                }

                Text {
                    text: "LISTENERS & CONNECTIONS"
                    color: Theme.faint
                    font.pixelSize: 9
                    font.letterSpacing: 1
                    Layout.topMargin: 8
                }

                Repeater {
                    model: app.processDetail.open_sockets || []

                    Rectangle {
                        required property var modelData

                        Layout.fillWidth: true
                        Layout.preferredHeight: 32
                        color: Theme.raised
                        radius: Theme.radiusSm

                        RowLayout {
                            anchors.fill: parent
                            anchors.margins: 8

                            Text {
                                text: modelData.protocol + " " + modelData.family
                                color: Theme.dim
                                font.pixelSize: 9
                                Layout.preferredWidth: 70
                            }

                            Text {
                                text: app.endpoint(modelData.local_addr, modelData.local_port)
                                color: Theme.text
                                font.pixelSize: 10
                                font.family: Theme.mono
                                Layout.fillWidth: true
                            }

                            Text {
                                text: modelData.remote_port ? app.endpoint(modelData.remote_addr, modelData.remote_port) : modelData.path || "—"
                                color: Theme.dim
                                font.pixelSize: 10
                                font.family: Theme.mono
                                Layout.fillWidth: true
                            }

                            Text {
                                text: modelData.state || ""
                                color: Theme.faint
                                font.pixelSize: 9
                            }

                        }

                    }

                }

                Text {
                    text: "EFFECTIVE CAPABILITIES · " + ((app.processDetail.capabilities || []).length || "none")
                    color: Theme.faint
                    font.pixelSize: 9
                    font.letterSpacing: 1
                    Layout.topMargin: 8
                }

                Flow {
                    Layout.fillWidth: true
                    spacing: 6

                    Repeater {
                        model: app.processDetail.capabilities || []

                        Rectangle {
                            required property var modelData

                            width: capText.implicitWidth + 14
                            height: 25
                            radius: 12
                            color: Theme.raised

                            Text {
                                id: capText

                                anchors.centerIn: parent
                                text: modelData
                                color: app.capabilityColor(modelData)
                                font.pixelSize: 9
                                font.family: Theme.mono
                            }

                        }

                    }

                }

                Text {
                    text: "NAMESPACES"
                    color: Theme.faint
                    font.pixelSize: 9
                    font.letterSpacing: 1
                    Layout.topMargin: 8
                }

                Flow {
                    Layout.fillWidth: true
                    spacing: 6

                    Repeater {
                        model: app.processDetail.namespaces || []

                        Rectangle {
                            required property var modelData

                            width: nsText.implicitWidth + 14
                            height: 25
                            radius: 12
                            color: Theme.raised

                            Text {
                                id: nsText

                                anchors.centerIn: parent
                                text: modelData.type + ":" + modelData.inode
                                color: Theme.dim
                                font.pixelSize: 9
                                font.family: Theme.mono
                            }

                        }

                    }

                }

            }

        }

        footer: DialogButtonBox {
            standardButtons: DialogButtonBox.Close
            onRejected: processDialog.close()
        }

    }

    Dialog {
        id: softwareDialog

        width: Math.min(840, app.width - 80)
        height: Math.min(700, app.height - 80)
        anchors.centerIn: parent
        modal: true
        title: app.softwareDetail.name || app.softwareDetail.identity || "Installed software"

        background: Rectangle {
            color: Theme.surface
            radius: Theme.radius
            border.width: 1
            border.color: Theme.divider
        }

        header: Rectangle {
            width: parent.width
            height: 68
            color: Theme.raised

            RowLayout {
                anchors.fill: parent
                anchors.leftMargin: 17
                anchors.rightMargin: 17

                Column {
                    Layout.fillWidth: true

                    Text {
                        text: softwareDialog.title
                        color: Theme.text
                        font.pixelSize: 17
                        font.weight: 650
                        font.family: Theme.mono
                    }

                    Text {
                        text: (app.softwareDetail.version || "unknown") + (app.softwareDetail.architecture ? " · " + app.softwareDetail.architecture : "")
                        color: Theme.dim
                        font.pixelSize: 10
                        font.family: Theme.mono
                    }

                }

                Text {
                    text: String(app.softwareDetail.package_type || "unknown").toUpperCase()
                    color: Theme.cyan
                    font.pixelSize: 10
                    font.weight: 700
                    font.letterSpacing: 1
                }

            }

        }

        contentItem: ScrollView {
            clip: true
            contentWidth: availableWidth

            ColumnLayout {
                width: softwareDialog.availableWidth
                spacing: 10

                Text {
                    visible: app.softwareDetail.error !== undefined
                    text: app.softwareDetail.error || "Software detail unavailable"
                    color: Theme.bad
                    font.pixelSize: 11
                    font.family: Theme.mono
                    wrapMode: Text.Wrap
                    Layout.fillWidth: true
                }

                Text {
                    visible: app.softwareDetail.error === undefined
                    text: app.softwareDetail.description || "No description reported by the package manager."
                    color: Theme.text
                    font.pixelSize: 12
                    wrapMode: Text.Wrap
                    Layout.fillWidth: true
                }

                GridLayout {
                    visible: app.softwareDetail.error === undefined
                    columns: 4
                    Layout.fillWidth: true
                    columnSpacing: 10
                    rowSpacing: 8

                    Repeater {
                        model: [{
                            "l": "Reason",
                            "v": app.softwareDetail.install_reason || "unknown"
                        }, {
                            "l": "Origin",
                            "v": app.softwareDetail.origin || "unknown"
                        }, {
                            "l": "Installed size",
                            "v": app.bytes(app.softwareDetail.installed_size_bytes)
                        }, {
                            "l": "Files",
                            "v": String(app.softwareDetail.file_count || 0) + (app.softwareDetail.files_truncated ? "+" : "")
                        }, {
                            "l": "Packager",
                            "v": app.softwareDetail.packager || "unknown"
                        }, {
                            "l": "Build date",
                            "v": app.softwareDetail.build_date || "unknown"
                        }, {
                            "l": "Installed",
                            "v": app.softwareDetail.installed_at || "unknown"
                        }, {
                            "l": "Identity",
                            "v": app.softwareDetail.identity || "unknown"
                        }]

                        Rectangle {
                            required property var modelData

                            Layout.fillWidth: true
                            Layout.preferredHeight: 62
                            color: Theme.raised
                            radius: Theme.radiusSm

                            Column {
                                anchors.centerIn: parent
                                width: parent.width - 14

                                Text {
                                    anchors.horizontalCenter: parent.horizontalCenter
                                    text: modelData.l.toUpperCase()
                                    color: Theme.faint
                                    font.pixelSize: 8
                                }

                                Text {
                                    width: parent.width
                                    horizontalAlignment: Text.AlignHCenter
                                    text: modelData.v
                                    color: Theme.text
                                    font.pixelSize: 10
                                    font.family: Theme.mono
                                    elide: Text.ElideMiddle
                                }

                            }

                        }

                    }

                }

                Text {
                    visible: (app.softwareDetail.depends_on || []).length > 0
                    text: "DEPENDENCY CLOSURE EDGES"
                    color: Theme.faint
                    font.pixelSize: 9
                    font.letterSpacing: 1
                    Layout.topMargin: 5
                }

                Flow {
                    visible: (app.softwareDetail.depends_on || []).length > 0
                    Layout.fillWidth: true
                    spacing: 6

                    Repeater {
                        model: app.softwareDetail.depends_on || []

                        Rectangle {
                            required property var modelData

                            width: dependencyText.implicitWidth + 14
                            height: 25
                            radius: 12
                            color: Theme.raised

                            Text {
                                id: dependencyText

                                anchors.centerIn: parent
                                text: modelData
                                color: Theme.cyan
                                font.pixelSize: 9
                                font.family: Theme.mono
                            }

                        }

                    }

                }

                Text {
                    visible: (app.softwareDetail.required_by || []).length > 0
                    text: "REQUIRED BY"
                    color: Theme.faint
                    font.pixelSize: 9
                    font.letterSpacing: 1
                    Layout.topMargin: 5
                }

                Flow {
                    visible: (app.softwareDetail.required_by || []).length > 0
                    Layout.fillWidth: true
                    spacing: 6

                    Repeater {
                        model: app.softwareDetail.required_by || []

                        Rectangle {
                            required property var modelData

                            width: requiredText.implicitWidth + 14
                            height: 25
                            radius: 12
                            color: Theme.raised

                            Text {
                                id: requiredText

                                anchors.centerIn: parent
                                text: modelData
                                color: Theme.purple
                                font.pixelSize: 9
                                font.family: Theme.mono
                            }

                        }

                    }

                }

                Text {
                    visible: (app.softwareDetail.provides || []).length > 0
                    text: "PROVIDES"
                    color: Theme.faint
                    font.pixelSize: 9
                    font.letterSpacing: 1
                    Layout.topMargin: 5
                }

                Text {
                    visible: (app.softwareDetail.provides || []).length > 0
                    Layout.fillWidth: true
                    text: (app.softwareDetail.provides || []).join(" · ")
                    color: Theme.dim
                    font.pixelSize: 10
                    font.family: Theme.mono
                    wrapMode: Text.Wrap
                }

                Text {
                    visible: (app.softwareDetail.files || []).length > 0
                    text: "FILES"
                    color: Theme.faint
                    font.pixelSize: 9
                    font.letterSpacing: 1
                    Layout.topMargin: 5
                }

                Repeater {
                    model: (app.softwareDetail.files || []).slice(0, 80)

                    Text {
                        required property var modelData

                        Layout.fillWidth: true
                        text: modelData
                        color: Theme.dim
                        font.pixelSize: 9
                        font.family: Theme.mono
                        elide: Text.ElideMiddle
                    }

                }

                Text {
                    visible: (app.softwareDetail.file_count || 0) > 80
                    text: "Showing 80 of " + app.softwareDetail.file_count + " paths"
                    color: Theme.faint
                    font.pixelSize: 9
                    font.family: Theme.mono
                }

            }

        }

        footer: DialogButtonBox {
            standardButtons: DialogButtonBox.Close
            onRejected: softwareDialog.close()
        }

    }

    Dialog {
        id: updateDialog

        width: Math.min(820, app.width - 80)
        height: Math.min(700, app.height - 80)
        anchors.centerIn: parent
        modal: true
        onOpened: {
            overrideField.text = "";
            overrideToggle.checked = false;
        }
        onClosed: machine.refreshOverview()

        background: Rectangle {
            color: Theme.surface
            radius: Theme.radius
            border.width: 1
            border.color: Theme.divider
        }

        header: Rectangle {
            width: parent.width
            height: 62
            color: Theme.raised

            Column {
                anchors.left: parent.left
                anchors.leftMargin: 16
                anchors.verticalCenter: parent.verticalCenter
                spacing: 2

                Text {
                    text: app.selectedUpdate.package_name || "Update"
                    color: Theme.text
                    font.pixelSize: 16
                    font.weight: 650
                    font.family: Theme.mono
                }

                Text {
                    text: (app.selectedUpdate.current_version || "?") + "  →  " + (app.selectedUpdate.available_version || "?") + " · " + String(app.selectedUpdate.package_type || "").toUpperCase()
                    color: Theme.dim
                    font.pixelSize: 11
                    font.family: Theme.mono
                }

            }

            Text {
                anchors.right: parent.right
                anchors.rightMargin: 16
                anchors.verticalCenter: parent.verticalCenter
                text: String(app.selectedUpdate.severity || "unknown").toUpperCase()
                color: app.selectedUpdate.severity === "critical" ? Theme.bad : app.selectedUpdate.severity === "high" || app.selectedUpdate.severity === "important" ? Theme.warn : Theme.faint
                font.pixelSize: 9
                font.weight: 700
                font.letterSpacing: 1
            }

        }

        contentItem: ScrollView {
            clip: true

            ColumnLayout {
                width: updateDialog.availableWidth
                spacing: 10

                Text {
                    text: "MUTATION PIPELINE"
                    color: Theme.faint
                    font.pixelSize: 9
                    font.letterSpacing: 1
                }

                RowLayout {
                    Layout.fillWidth: true
                    spacing: 0

                    Repeater {
                        model: [{
                            "label": "Discovered",
                            "detail": app.selectedUpdate.repository_source || "reported by the scanner"
                        }, {
                            "label": "Checked",
                            "detail": app.selectedUpdate.package_type === "pacman" ? "closure · hashes · package signatures" : "closure resolved · hashes pinned · OSV"
                        }, {
                            "label": "Authorized",
                            "detail": app.selectedUpdate.package_type === "pacman" ? "mutation envelope signed for this host" : "capability minted for this host"
                        }, {
                            "label": "Installed",
                            "detail": "helper verified and executed"
                        }]

                        Item {
                            required property var modelData
                            required property int index

                            Layout.fillWidth: true
                            Layout.preferredHeight: 66

                            Rectangle {
                                id: dot

                                width: 15
                                height: 15
                                radius: 8
                                anchors.horizontalCenter: parent.horizontalCenter
                                anchors.top: parent.top
                                anchors.topMargin: 4
                                color: app.stageColor(app.stageState(index))
                                opacity: app.stageState(index) === "idle" ? 0.35 : 1
                            }

                            Rectangle {
                                visible: index < 3
                                height: 2
                                anchors.left: dot.right
                                anchors.right: parent.right
                                anchors.verticalCenter: dot.verticalCenter
                                color: app.stageColor(app.stageState(index + 1))
                                opacity: 0.35
                            }

                            Text {
                                anchors.horizontalCenter: parent.horizontalCenter
                                anchors.top: dot.bottom
                                anchors.topMargin: 7
                                text: modelData.label
                                color: Theme.text
                                font.pixelSize: 11
                                font.weight: 600
                            }

                            Text {
                                anchors.horizontalCenter: parent.horizontalCenter
                                anchors.bottom: parent.bottom
                                width: parent.width - 12
                                horizontalAlignment: Text.AlignHCenter
                                text: modelData.detail
                                color: Theme.faint
                                font.pixelSize: 8
                                elide: Text.ElideRight
                            }

                        }

                    }

                }

                Text {
                    visible: (app.selectedUpdate.cve_list || []).length > 0
                    text: "ADVISORIES"
                    color: Theme.faint
                    font.pixelSize: 9
                    font.letterSpacing: 1
                    Layout.topMargin: 6
                }

                Flow {
                    Layout.fillWidth: true
                    spacing: 6

                    Repeater {
                        model: app.selectedUpdate.cve_list || []

                        Rectangle {
                            required property var modelData

                            width: cveText.implicitWidth + 14
                            height: 25
                            radius: 12
                            color: Theme.redDim

                            Text {
                                id: cveText

                                anchors.centerIn: parent
                                text: modelData
                                color: Theme.bad
                                font.pixelSize: 9
                                font.family: Theme.mono
                            }

                        }

                    }

                }

                Text {
                    visible: app.approval.request_id !== undefined || app.approval.error !== undefined
                    text: "GATE EVIDENCE"
                    color: Theme.faint
                    font.pixelSize: 9
                    font.letterSpacing: 1
                    Layout.topMargin: 6
                }

                GridLayout {
                    visible: app.approval.request_id !== undefined
                    columns: 4
                    Layout.fillWidth: true
                    columnSpacing: 10
                    rowSpacing: 8

                    Repeater {
                        model: [{
                            "l": "Closure",
                            "v": String(app.approval.closure_size !== undefined ? app.approval.closure_size : "—")
                        }, {
                            "l": "OSV",
                            "v": app.approval.osv_status || "—"
                        }, {
                            "l": "Vulnerabilities",
                            "v": String(app.approval.osv_vuln_count !== undefined ? app.approval.osv_vuln_count : "—")
                        }, {
                            "l": "Verified artifacts",
                            "v": String(app.verifiedActionCount())
                        }, {
                            "l": "Decision",
                            "v": app.approvalResult().decision || "—"
                        }, {
                            "l": "Exit code",
                            "v": String(app.approvalResult().exit_code !== undefined ? app.approvalResult().exit_code : "—")
                        }, {
                            "l": "Executed",
                            "v": app.approvalResult().executed ? "yes" : "no"
                        }, {
                            "l": "Request",
                            "v": String(app.approval.request_id || "—").substring(0, 12)
                        }]

                        Rectangle {
                            required property var modelData

                            Layout.fillWidth: true
                            Layout.preferredHeight: 58
                            color: Theme.raised
                            radius: Theme.radiusSm

                            Column {
                                anchors.centerIn: parent

                                Text {
                                    anchors.horizontalCenter: parent.horizontalCenter
                                    text: modelData.l.toUpperCase()
                                    color: Theme.faint
                                    font.pixelSize: 8
                                }

                                Text {
                                    anchors.horizontalCenter: parent.horizontalCenter
                                    text: modelData.v
                                    color: Theme.text
                                    font.pixelSize: 11
                                    font.family: Theme.mono
                                }

                            }

                        }

                    }

                }

                Text {
                    visible: text.length > 0
                    Layout.fillWidth: true
                    text: app.approval.error || app.approvalResult().reason || ""
                    color: app.approval.error ? Theme.bad : Theme.dim
                    font.pixelSize: 11
                    font.family: Theme.mono
                    wrapMode: Text.Wrap
                }

                Rectangle {
                    visible: app.approvalBlockReason(app.selectedUpdate).length > 0
                    Layout.fillWidth: true
                    Layout.preferredHeight: blockText.implicitHeight + 22
                    color: Theme.raised
                    radius: Theme.radiusSm

                    Text {
                        id: blockText

                        anchors.fill: parent
                        anchors.margins: 11
                        text: app.approvalBlockReason(app.selectedUpdate)
                        color: Theme.dim
                        font.pixelSize: 11
                        wrapMode: Text.Wrap
                    }

                }

                Rectangle {
                    visible: app.approvalReasonRequired(app.selectedUpdate)
                    Layout.fillWidth: true
                    Layout.preferredHeight: pacmanEvidenceText.implicitHeight + 22
                    color: Theme.raised
                    radius: Theme.radiusSm
                    border.width: 1
                    border.color: Theme.warn

                    Text {
                        id: pacmanEvidenceText

                        anchors.fill: parent
                        anchors.margins: 11
                        text: "OSV has no Arch Linux package mapping in this path. RedFlag can resolve and cryptographically verify the exact pacman transaction, but it cannot call the dependency closure advisory-clear. State why you accept that uncertainty; the root helper records the reason beside the request and authorization in its journal."
                        color: Theme.warn
                        font.pixelSize: 11
                        wrapMode: Text.Wrap
                    }

                }

                CheckBox {
                    id: overrideToggle

                    visible: !app.approvalReasonRequired(app.selectedUpdate) && app.approvalCanOverride() && app.approvalBlockReason(app.selectedUpdate).length === 0 && !app.approvalResult().executed
                    text: "Override this advisory refusal with recorded operator intent"
                    onToggled: {
                        if (!checked) {
                            overrideField.text = "";
                        }
                    }

                    contentItem: Text {
                        text: parent.text
                        color: parent.checked ? Theme.warn : Theme.dim
                        font.pixelSize: 11
                        leftPadding: parent.indicator.width + parent.spacing
                    }

                }

                TextArea {
                    id: overrideField

                    visible: app.approvalReasonRequired(app.selectedUpdate) || overrideToggle.checked
                    Layout.fillWidth: true
                    Layout.preferredHeight: 76
                    placeholderText: app.approvalReasonRequired(app.selectedUpdate) ? "Why this exact signed pacman transaction should proceed without OSV advisory coverage." : "Why the operator is breaking glass. The root helper records this reason beside the authorization in durable history."
                    wrapMode: TextEdit.Wrap
                    color: Theme.text
                    placeholderTextColor: Theme.faint

                    background: Rectangle {
                        color: Theme.surface
                        border.width: 1
                        border.color: overrideField.activeFocus ? Theme.red : Theme.divider
                        radius: Theme.radiusSm
                    }

                }

                Text {
                    Layout.fillWidth: true
                    Layout.topMargin: 4
                    text: app.selectedUpdate.package_type === "pacman" ? "The Agent resolves a private pacman transaction and downloads every exact archive and detached signature. The root helper stages and verifies identity, hashes, Arch signatures, and forward-only versions before signing the mutation envelope, then repeats those checks before pacman runs. OSV has no Arch mapping here, so acceptance stays explicit. This window never touches pacman." : "Approval resolves the dependency closure, pins every artifact hash it can, checks the set against OSV, mints an Ed25519 capability bound to this host, and hands it to the privileged helper. The helper verifies independently. This window never touches a package manager."
                    color: Theme.faint
                    font.pixelSize: 10
                    wrapMode: Text.Wrap
                }

            }

        }

        footer: DialogButtonBox {
            Button {
                text: machine.approvalRunning ? "Authorizing…" : (app.approvalResult().executed ? "Installed" : app.approvalReasonRequired(app.selectedUpdate) ? "Approve with reason and install" : overrideToggle.checked ? "Override and install" : "Approve and install")
                enabled: !machine.approvalRunning && app.approvalBlockReason(app.selectedUpdate).length === 0 && !app.approvalResult().executed && (!(app.approvalReasonRequired(app.selectedUpdate) || overrideToggle.checked) || overrideField.text.trim().length > 0)
                DialogButtonBox.buttonRole: DialogButtonBox.AcceptRole
                onClicked: machine.approveUpdate(app.selectedUpdate.package_type || "", app.selectedUpdate.package_name || "", app.selectedUpdate.available_version || "", app.approvalReasonRequired(app.selectedUpdate) || overrideToggle.checked ? overrideField.text.trim() : "")

                background: Rectangle {
                    color: !parent.enabled ? Theme.raised : parent.pressed ? Qt.darker(Theme.red, 1.15) : Theme.red
                    radius: Theme.radiusSm
                }

                contentItem: Text {
                    text: parent.text
                    color: parent.enabled ? "white" : Theme.faint
                    horizontalAlignment: Text.AlignHCenter
                    verticalAlignment: Text.AlignVCenter
                    font.pixelSize: 11
                    font.weight: 650
                }

            }

            Button {
                text: "Close"
                DialogButtonBox.buttonRole: DialogButtonBox.RejectRole
                onClicked: updateDialog.close()

                background: Rectangle {
                    color: parent.pressed ? Theme.hover : "transparent"
                    border.width: 1
                    border.color: Theme.divider
                    radius: Theme.radiusSm
                }

                contentItem: Text {
                    text: parent.text
                    color: Theme.dim
                    horizontalAlignment: Text.AlignHCenter
                    verticalAlignment: Text.AlignVCenter
                    font.pixelSize: 11
                }

            }

        }

    }

}
