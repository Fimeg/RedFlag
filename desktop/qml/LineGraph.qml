import QtQuick

Canvas {
    id: root
    property var values: []
    property real ceiling: 100
    property color lineColor: Theme.red
    property color fillColor: Theme.tint(lineColor, 0.12)
    property bool grid: true

    onValuesChanged: requestPaint()
    onWidthChanged: requestPaint()
    onHeightChanged: requestPaint()
    onCeilingChanged: requestPaint()

    onPaint: {
        const ctx = getContext("2d")
        ctx.reset()
        if (grid) {
            ctx.strokeStyle = Theme.divider
            ctx.lineWidth = 1
            for (let y = 0; y <= 4; ++y) {
                ctx.beginPath(); ctx.moveTo(0, y * height / 4); ctx.lineTo(width, y * height / 4); ctx.stroke()
            }
        }
        if (!values || values.length < 2) return
        const top = Math.max(1, ceiling)
        const step = width / Math.max(1, values.length - 1)
        ctx.beginPath()
        for (let i = 0; i < values.length; ++i) {
            const x = i * step
            const y = height - Math.min(top, Math.max(0, Number(values[i]))) / top * height
            if (i === 0) ctx.moveTo(x, y); else ctx.lineTo(x, y)
        }
        ctx.lineTo(width, height); ctx.lineTo(0, height); ctx.closePath()
        ctx.fillStyle = fillColor; ctx.fill()
        ctx.beginPath()
        for (let j = 0; j < values.length; ++j) {
            const x2 = j * step
            const y2 = height - Math.min(top, Math.max(0, Number(values[j]))) / top * height
            if (j === 0) ctx.moveTo(x2, y2); else ctx.lineTo(x2, y2)
        }
        ctx.strokeStyle = lineColor; ctx.lineWidth = 2; ctx.stroke()
    }
}
