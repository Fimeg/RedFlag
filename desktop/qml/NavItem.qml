import QtQuick

Rectangle {
    id: root
    required property string label
    property string badge: ""
    property bool selected: false
    signal clicked()

    width: parent ? parent.width : 180
    height: 38
    radius: Theme.radiusSm
    color: selected ? Theme.redDim : mouse.containsMouse ? Theme.hover : "transparent"

    Rectangle {
        width: 3
        height: 18
        radius: 2
        anchors.left: parent.left
        anchors.leftMargin: 2
        anchors.verticalCenter: parent.verticalCenter
        color: root.selected ? Theme.red : "transparent"
    }
    Text {
        anchors.left: parent.left
        anchors.leftMargin: 16
        anchors.verticalCenter: parent.verticalCenter
        text: root.label
        color: root.selected ? Theme.text : Theme.dim
        font.pixelSize: Theme.fontBody
        font.weight: root.selected ? 600 : 450
    }
    Rectangle {
        visible: root.badge.length > 0
        anchors.right: parent.right
        anchors.rightMargin: 10
        anchors.verticalCenter: parent.verticalCenter
        width: Math.max(20, badgeText.implicitWidth + 10)
        height: 20
        radius: 10
        color: Theme.tint(root.selected ? Theme.red : Theme.faint, 0.22)
        Text {
            id: badgeText
            anchors.centerIn: parent
            text: root.badge
            color: root.selected ? Theme.red : Theme.dim
            font.pixelSize: 10
            font.weight: 700
        }
    }
    MouseArea { id: mouse; anchors.fill: parent; hoverEnabled: true; onClicked: root.clicked() }
}
