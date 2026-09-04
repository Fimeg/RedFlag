pragma Singleton
import QtQuick

QtObject {
    property bool dark: true

    readonly property color window: dark ? "#0b0e13" : "#f3f4f6"
    readonly property color rail: dark ? "#10141b" : "#ffffff"
    readonly property color surface: dark ? "#151a22" : "#ffffff"
    readonly property color raised: dark ? "#1b222d" : "#f8fafc"
    readonly property color hover: dark ? "#222b38" : "#eef2f6"
    readonly property color divider: dark ? "#252d39" : "#dce1e7"
    readonly property color text: dark ? "#eef1f5" : "#171a1f"
    readonly property color dim: dark ? "#99a2af" : "#5f6874"
    readonly property color faint: dark ? "#66707d" : "#8a939e"
    readonly property color red: "#e23b3b"
    readonly property color redDim: dark ? "#3a1a1e" : "#fee2e2"
    readonly property color good: "#32bd74"
    readonly property color warn: "#e5a936"
    readonly property color bad: "#ef5350"
    readonly property color blue: "#4d8dff"
    readonly property color purple: "#ad7aff"
    readonly property color cyan: "#30b8c9"

    readonly property int railWidth: 216
    readonly property int headerHeight: 72
    readonly property int radius: 10
    readonly property int radiusSm: 6
    readonly property int spacing: 14
    readonly property int spacingSm: 8
    readonly property int fontTitle: 23
    readonly property int fontHeading: 17
    readonly property int fontBody: 13
    readonly property int fontMeta: 11
    readonly property string mono: {
        const wanted = ["JetBrainsMono NF", "Adwaita Mono", "DejaVu Sans Mono"]
        const installed = Qt.fontFamilies()
        for (let i = 0; i < wanted.length; ++i)
            if (installed.indexOf(wanted[i]) >= 0) return wanted[i]
        return "monospace"
    }

    function tint(color, alpha) { return Qt.rgba(color.r, color.g, color.b, alpha) }
    function pressure(percent) { return percent >= 90 ? bad : percent >= 75 ? warn : good }
}
