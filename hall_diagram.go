package main

import (
    "image"
    "image/color"
    "image/draw"
    "image/png"
    "log"
    "math/rand"
    "net/http"
    "strconv"
    "time"

    "golang.org/x/image/font"
    "golang.org/x/image/font/basicfont"
    "golang.org/x/image/math/fixed"
)

func HallDiagramHandler(w http.ResponseWriter, r *http.Request) {
    q := r.URL.Query()
    rows := parseIntOrDefault(q.Get("rows"), 8)
    cols := parseIntOrDefault(q.Get("cols"), 12)
    occupiedPct := parseIntOrDefault(q.Get("occupiedPct"), 30)

    seatSize := 36
    seatGap := 6
    margin := 20
    legendWidth := 220

    width := margin*2 + cols*(seatSize+seatGap) - seatGap + legendWidth
    height := margin*2 + rows*(seatSize+seatGap) - seatGap

    img := image.NewRGBA(image.Rect(0, 0, width, height))
    draw.Draw(img, img.Bounds(), &image.Uniform{color.White}, image.Point{}, draw.Src)

    colFree := color.RGBA{0, 180, 0, 255}
    colOccupied := color.RGBA{200, 0, 0, 255}
    colVipFree := color.RGBA{0, 70, 200, 255}
    colVipOcc := color.RGBA{150, 50, 150, 255}
    colSeatBorder := color.RGBA{30, 30, 30, 255}
    colScreen := color.RGBA{120, 120, 120, 255}
    colText := color.Black

    rand.Seed(time.Now().UnixNano())

    screenLeft := margin
    screenRight := margin + cols*(seatSize+seatGap) - seatGap
    screenRect := image.Rect(screenLeft, margin, screenRight, margin+16)
    draw.Draw(img, screenRect, &image.Uniform{colScreen}, image.Point{}, draw.Src)
    drawString(img, screenLeft, margin-2, "SCREEN", colText)

    startY := margin + 24
    for row := 0; row < rows; row++ {
        rowLetter := string('A' + rune(row))
        for col := 0; col < cols; col++ {
            x := margin + col*(seatSize+seatGap)
            y := startY + row*(seatSize+seatGap)
            rect := image.Rect(x, y, x+seatSize, y+seatSize)

            isVIP := row >= rows-2
            occupied := rand.Intn(100) < occupiedPct

            var fill color.Color
            switch {
            case isVIP && occupied:
                fill = colVipOcc
            case isVIP:
                fill = colVipFree
            case occupied:
                fill = colOccupied
            default:
                fill = colFree
            }
            draw.Draw(img, rect, &image.Uniform{fill}, image.Point{}, draw.Src)
            drawRectBorder(img, rect, colSeatBorder)

            seatLabel := rowLetter + strconv.Itoa(col+1)
            tx := x + 4
            ty := y + seatSize/2 + 6
            drawString(img, tx, ty, seatLabel, colText)
        }
    }

    legendX := margin + cols*(seatSize+seatGap) - seatGap + 20
    legendY := margin + 10
    legendItemH := 28

    drawLegendItem(img, legendX, legendY+0*legendItemH, "Free", colFree)
    drawLegendItem(img, legendX, legendY+1*legendItemH, "Occupied", colOccupied)
    drawLegendItem(img, legendX, legendY+2*legendItemH, "VIP Free", colVipFree)
    drawLegendItem(img, legendX, legendY+3*legendItemH, "VIP Occupied", colVipOcc)

    info := "Rows: " + strconv.Itoa(rows) + "  Cols: " + strconv.Itoa(cols)
    drawString(img, legendX, legendY+5*legendItemH, info, colText)

    w.Header().Set("Content-Type", "image/png")
    if err := png.Encode(w, img); err != nil {
        log.Printf("Ошибка PNG: %v", err)
        http.Error(w, "Ошибка генерации изображения", http.StatusInternalServerError)
    }
}

func parseIntOrDefault(s string, def int) int {
    if s == "" {
        return def
    }
    v, err := strconv.Atoi(s)
    if err != nil || v <= 0 {
        return def
    }
    return v
}

func drawRectBorder(img *image.RGBA, rect image.Rectangle, col color.Color) {
    for x := rect.Min.X; x < rect.Max.X; x++ {
        img.Set(x, rect.Min.Y, col)
        img.Set(x, rect.Max.Y-1, col)
    }
    for y := rect.Min.Y; y < rect.Max.Y; y++ {
        img.Set(rect.Min.X, y, col)
        img.Set(rect.Max.X-1, y, col)
    }
}

func drawString(img *image.RGBA, x, y int, label string, col color.Color) {
    point := fixed.Point26_6{
        X: fixed.I(x),
        Y: fixed.I(y),
    }
    d := &font.Drawer{
        Dst:  img,
        Src:  image.NewUniform(col),
        Face: basicfont.Face7x13,
        Dot:  point,
    }
    d.DrawString(label)
}

func drawLegendItem(img *image.RGBA, x, y int, text string, fill color.Color) {
    box := image.Rect(x, y, x+18, y+18)
    draw.Draw(img, box, &image.Uniform{fill}, image.Point{}, draw.Src)
    drawRectBorder(img, box, color.RGBA{30, 30, 30, 255})
    drawString(img, x+24, y+14, text, color.Black)
}
