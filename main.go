package main

import (
	"bytes"
	_ "embed"
	"fmt"
	"image"
	"image/color"
	_ "image/jpeg"
	_ "image/png"
	"log"
	"os"
	"path/filepath"
	"strings"

	runtime "github.com/Iteranya/ToxicEbitenGamejam2026/hikarin_runtime"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"
)

//go:embed story.json
var storyJSON []byte

// --- ASSET MANAGEMENT ---
var (
	AssetsPath = "assets/" // Path prefix for all dynamic assets
	imageCache = make(map[string]*ebiten.Image)
)

// --- FONTS ---
var (
	dialogueFace *text.GoTextFace
	nameFace     *text.GoTextFace
)

// --- THEME COLORS ---
var (
	colorPanelBg   = color.RGBA{15, 15, 20, 220}
	colorPanelLine = color.RGBA{220, 180, 80, 255}
	colorNameBg    = color.RGBA{220, 180, 80, 255}
	colorNameText  = color.RGBA{20, 20, 25, 255}
	colorText      = color.RGBA{240, 240, 245, 255}
	colorChoice    = color.RGBA{25, 30, 45, 230}
	colorChoiceHov = color.RGBA{60, 85, 130, 255}
)

func init() {
	// Initialize Modern Fonts
	regSource, _ := text.NewGoTextFaceSource(bytes.NewReader(goregular.TTF))
	boldSource, _ := text.NewGoTextFaceSource(bytes.NewReader(gobold.TTF))

	dialogueFace = &text.GoTextFace{Source: regSource, Size: 24}
	nameFace = &text.GoTextFace{Source: boldSource, Size: 26}
}

type Game struct {
	vn *runtime.VisualNovelRuntime
}

func (g *Game) Update() error {
	sw, sh := 1280.0, 720.0
	blockW, blockH := sw/16.0, sh/9.0

	switch g.vn.GetState() {
	case runtime.StateWaiting:
		if inpututil.IsKeyJustPressed(ebiten.KeySpace) ||
			inpututil.IsKeyJustPressed(ebiten.KeyEnter) ||
			inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
			g.vn.Advance()
		}

	case runtime.StateChoice:
		mx, my := ebiten.CursorPosition()
		mouseX, mouseY := float64(mx), float64(my)
		totalChoices := len(g.vn.Choices)

		if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
			for i, ch := range g.vn.Choices {
				cx, cy, cw, chH := getChoiceBounds(i, totalChoices, blockW, blockH)
				if mouseX >= cx && mouseX <= cx+cw && mouseY >= cy && mouseY <= cy+chH {
					g.vn.SelectChoice(ch.Label)
					break
				}
			}
		}

		// Keyboard quick selection
		for i, ch := range g.vn.Choices {
			if i >= 9 {
				break
			}
			if inpututil.IsKeyJustPressed(ebiten.Key1 + ebiten.Key(i)) {
				g.vn.SelectChoice(ch.Label)
				break
			}
		}

	case runtime.StateEnded:
		if inpututil.IsKeyJustPressed(ebiten.KeyR) {
			vn := runtime.NewRuntime()
			vn.LoadScript(storyJSON, nil, nil)
			vn.Start("")
			g.vn = vn
		}
	}
	return nil
}

func (g *Game) Draw(screen *ebiten.Image) {
	screen.Fill(color.RGBA{10, 10, 10, 255})

	// 1. DYNAMIC BACKGROUND
	if g.vn.Background != "" {
		bgImg := getImage(AssetsPath + g.vn.Background)
		// 1x1 logical framework representing the whole screen
		drawGridImage(screen, bgImg, 1, 1, 1, 1, 1, 1)
	}

	// 2. DYNAMIC CHARACTER SPRITES
	// This relies on the dynamic grid ratios provided by the JS payload equivalent
	for _, sprite := range g.vn.ActiveSprites {
		img := getImage(AssetsPath + sprite.FinalLocation)
		drawGridImage(screen, img, sprite.Column, sprite.Row, sprite.WRatio, sprite.HRatio, sprite.WFrameRatio, sprite.HFrameRatio)
	}

	sw, sh := 1280.0, 720.0
	blockW, blockH := sw/16.0, sh/9.0

	switch g.vn.GetState() {
	case runtime.StateWaiting:
		// Dialogue Box shows during normal script lines
		drawDialogueBox(screen, g.vn.DialogueSpeaker, g.vn.DialogueText)

	case runtime.StateChoice:
		// Dialogue box is HIDDEN during choices, rendering only the choice layout
		mx, my := ebiten.CursorPosition()
		totalChoices := len(g.vn.Choices)

		for i, ch := range g.vn.Choices {
			cx, cy, cw, chH := getChoiceBounds(i, totalChoices, blockW, blockH)

			bgCol := colorChoice
			if float64(mx) >= cx && float64(mx) <= cx+cw && float64(my) >= cy && float64(my) <= cy+chH {
				bgCol = colorChoiceHov
			}

			drawUIBox(screen, cx, cy, cw, chH, bgCol, colorPanelLine)

			choiceTxt := fmt.Sprintf("%d. %s", i+1, ch.Display)
			tw, th := text.Measure(choiceTxt, dialogueFace, dialogueFace.Size)
			drawText(screen, choiceTxt, dialogueFace, cx+(cw-tw)/2, cy+(chH-th)/2, colorText)
		}

	case runtime.StateEnded:
		drawDialogueBox(screen, "SYSTEM", "End of Script.\n(Press R to restart)")
	}
}

// --- HELPER FUNCTIONS ---

// getImage replicates JS `_verifyImageLoad`. It caches images, handles file loading,
// logs explicit error messages for misses, and returns a "MISSING" texture fallback.
func getImage(path string) *ebiten.Image {
	if path == "" {
		return nil
	}
	if img, ok := imageCache[path]; ok {
		return img
	}

	file, err := os.Open(path)
	if err != nil {
		log.Printf("[HikarinVN Error] Failed to load image: \"%s\"\nContext: Check if the file exists in your assets folder and the path is correct.\nError: %v", path, err)
		imageCache[path] = createMissingTexture(path) // ← pass path!
		return imageCache[path]
	}
	defer file.Close()

	img, _, err := image.Decode(file)
	if err != nil {
		log.Printf("[HikarinVN Error] Failed to decode image: \"%s\"\nError: %v", path, err)
		imageCache[path] = createMissingTexture(path) // ← pass path!
		return imageCache[path]
	}

	eImg := ebiten.NewImageFromImage(img)
	imageCache[path] = eImg
	return eImg
}

// createMissingTexture mimics the visual CSS properties of `.hvn-missing-asset`
// createMissingTexture mimics the visual CSS properties of `.hvn-missing-asset`
// and shows the actual filename that failed to load
func createMissingTexture(path string) *ebiten.Image {
	img := ebiten.NewImage(256, 256)
	img.Fill(color.RGBA{100, 0, 0, 50}) // Translucent red background

	vector.StrokeRect(img, 0, 0, 256, 256, 8, color.RGBA{100, 0, 0, 50}, true) // Red border

	// Draw "MISSING" label
	tw, th := text.Measure("MISSING", nameFace, nameFace.Size)
	drawText(img, "MISSING", nameFace, (256-tw)/2, (256-th)/2-20, color.RGBA{255, 255, 255, 255})

	// Draw the filename that failed
	filename := filepath.Base(path)
	fw, fh := text.Measure(filename, dialogueFace, dialogueFace.Size)
	drawText(img, filename, dialogueFace, (256-fw)/2, (256-fh)/2+20, color.RGBA{255, 255, 255, 255})

	return img
}

// drawGridImage accurately maps mathematical percentages provided by the runtime to screen space coordinates
func drawGridImage(screen, img *ebiten.Image, col, row, wRatio, hRatio, frameW, frameH float64) {
	if img == nil {
		return
	}
	sw, sh := 1280.0, 720.0

	// Dynamic calculation mapping exactly to JS's CSS percentage calculation logic
	targetW := (frameW / wRatio) * sw
	targetH := (frameH / hRatio) * sh
	posX := ((col - 1) / wRatio) * sw
	posY := ((row - 1) / hRatio) * sh

	imgW, imgH := float64(img.Bounds().Dx()), float64(img.Bounds().Dy())
	scaleX, scaleY := targetW/imgW, targetH/imgH

	op := &ebiten.DrawImageOptions{}
	op.GeoM.Scale(scaleX, scaleY)
	op.GeoM.Translate(posX, posY)
	screen.DrawImage(img, op)
}

func drawDialogueBox(screen *ebiten.Image, speakerName, dialogueText string) {
	sw, sh := 1280.0, 720.0
	blockW, blockH := sw/16.0, sh/9.0

	boxW, boxH := blockW*14, blockH*2.8
	boxX, boxY := blockW*1, blockH*5.8
	drawUIBox(screen, boxX, boxY, boxW, boxH, colorPanelBg, colorPanelLine)

	if speakerName != "" {
		nameW, nameH := blockW*3, blockH*0.8
		nameX, nameY := boxX, boxY-nameH+3
		drawUIBox(screen, nameX, nameY, nameW, nameH, colorNameBg, colorNameBg)

		nw, nh := text.Measure(speakerName, nameFace, nameFace.Size)
		drawText(screen, speakerName, nameFace, nameX+(nameW-nw)/2, nameY+(nameH-nh)/2, colorNameText)
	}

	padX, padY := blockW*0.5, blockH*0.4
	wrappedText := wrapText(dialogueText, boxW-(padX*2), dialogueFace)
	drawText(screen, wrappedText, dialogueFace, boxX+padX, boxY+padY, colorText)
}

// getChoiceBounds centers elements vertically regardless of amount (replicates JS align-items/justify-content config)
func getChoiceBounds(index, total int, blockW, blockH float64) (x, y, w, h float64) {
	frameW, frameH := 7.0, 0.9
	spacing := 1.2

	totalHeight := float64(total) * spacing
	startRow := (9.0 - totalHeight) / 2.0 // Flexbox center logic emulation

	col := 4.5 // Horizontally centered inside 16 units wide: (16 - 7)/2
	row := startRow + float64(index)*spacing

	return blockW * col, blockH * row, blockW * frameW, blockH * frameH
}

func drawUIBox(screen *ebiten.Image, x, y, w, h float64, bg, border color.Color) {
	vector.DrawFilledRect(screen, float32(x), float32(y), float32(w), float32(h), bg, true)
	vector.StrokeRect(screen, float32(x), float32(y), float32(w), float32(h), 3, border, true)
}

func drawText(screen *ebiten.Image, str string, face *text.GoTextFace, x, y float64, clr color.Color) {
	op := &text.DrawOptions{}
	op.GeoM.Translate(x, y)
	op.ColorScale.ScaleWithColor(clr)
	op.LineSpacing = face.Size * 1.5
	text.Draw(screen, str, face, op)
}

func wrapText(str string, maxWidth float64, face *text.GoTextFace) string {
	var lines []string
	words := strings.Split(str, " ")
	var currentLine string

	for _, word := range words {
		testLine := currentLine
		if testLine == "" {
			testLine = word
		} else {
			testLine += " " + word
		}

		w, _ := text.Measure(testLine, face, face.Size)
		if w > maxWidth && currentLine != "" {
			lines = append(lines, currentLine)
			currentLine = word
		} else {
			currentLine = testLine
		}
	}
	if currentLine != "" {
		lines = append(lines, currentLine)
	}
	return strings.Join(lines, "\n")
}

func (g *Game) Layout(outsideWidth, outsideHeight int) (int, int) {
	return 1280, 720
}

func main() {
	ebiten.SetWindowSize(1280, 720)
	ebiten.SetWindowTitle("Blood Slave Rapture")
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)

	vn := runtime.NewRuntime()
	if err := vn.LoadScript(storyJSON, nil, nil); err != nil {
		log.Fatal(err)
	}
	vn.Start("")

	g := &Game{vn: vn}
	if err := ebiten.RunGame(g); err != nil {
		log.Fatal(err)
	}
}
