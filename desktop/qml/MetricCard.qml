import QtQuick

Rectangle {
    required property string label
    required property string value
    property string detail: ""
    property color accent: Theme.text

    implicitWidth: 180
    implicitHeight: 96
    radius: Theme.radius
    color: Theme.surface
    border.width: 1
    border.color: Theme.divider

    Rectangle { width: 3; height: 28; radius: 2; color: parent.accent; anchors.left: parent.left; anchors.leftMargin: 13; anchors.top: parent.top; anchors.topMargin: 16 }
    Text { text: parent.label.toUpperCase(); color: Theme.faint; font.pixelSize: 10; font.letterSpacing: 1.1; anchors.left: parent.left; anchors.leftMargin: 26; anchors.top: parent.top; anchors.topMargin: 14 }
    Text { text: parent.value; color: parent.accent; font.pixelSize: 22; font.weight: 650; anchors.left: parent.left; anchors.leftMargin: 26; anchors.top: parent.top; anchors.topMargin: 32 }
    Text { text: parent.detail; color: Theme.dim; font.pixelSize: Theme.fontMeta; anchors.left: parent.left; anchors.leftMargin: 26; anchors.right: parent.right; anchors.rightMargin: 10; anchors.bottom: parent.bottom; anchors.bottomMargin: 11; elide: Text.ElideRight }
}
