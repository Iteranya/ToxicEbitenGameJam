package main

import (
	"bytes"
	"embed" // Changed from _ "embed" so we can use embed.FS
	"fmt"
	"image"
	"image/color"
	_ "image/jpeg"
	_ "image/png"
	"log"
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

//go:embed assets
var embeddedAssets embed.FS // This grabs your entire assets folder!

// --- ASSET MANAGEMENT ---
var (
	AssetsPath = "assets/" // Path prefix for all dynamic assets
	imageCache = make(map[string]*ebiten.Image)
)

// --- FONTS ---
var (
	dialogueFace *text.GoTextFace
	nameFace     *text.GoTextFace
	titleFace    *text.GoTextFace // Added a large font for the title
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

// --- GAME STATES ---
type GameState int

const (
	StateTitleScreen GameState = iota
	StatePlaying
)

func init() {
	// Initialize Modern Fonts
	regSource, _ := text.NewGoTextFaceSource(bytes.NewReader(goregular.TTF))
	boldSource, _ := text.NewGoTextFaceSource(bytes.NewReader(gobold.TTF))

	dialogueFace = &text.GoTextFace{Source: regSource, Size: 24}
	nameFace = &text.GoTextFace{Source: boldSource, Size: 26}
	titleFace = &text.GoTextFace{Source: boldSource, Size: 64} // Big Title Font!
}

type Game struct {
	vn    *runtime.VisualNovelRuntime
	state GameState // Tracks if we are in Menu or Game
}

func (g *Game) Update() error {
	sw, sh := 1280.0, 720.0
	blockW, blockH := sw/16.0, sh/9.0

	// --- TITLE SCREEN LOGIC ---
	if g.state == StateTitleScreen {
		mx, my := ebiten.CursorPosition()
		mouseX, mouseY := float64(mx), float64(my)

		// Begin Button Bounds
		btnW, btnH := blockW*4.0, blockH*1.5
		btnX, btnY := (sw-btnW)/2.0, sh*0.75

		// Click to begin
		if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
			if mouseX >= btnX && mouseX <= btnX+btnW && mouseY >= btnY && mouseY <= btnY+btnH {
				g.vn.Start("")
				g.state = StatePlaying
			}
		}

		// Also allow hitting Enter or Space to begin quickly
		if inpututil.IsKeyJustPressed(ebiten.KeySpace) || inpututil.IsKeyJustPressed(ebiten.KeyEnter) {
			g.vn.Start("")
			g.state = StatePlaying
		}

		return nil
	}

	// --- VISUAL NOVEL LOGIC ---
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
			// Restart game, back to title screen
			vn := runtime.NewRuntime()
			vn.LoadScript(storyJSON, nil, nil)
			g.vn = vn
			g.state = StateTitleScreen
		}
	}
	return nil
}

func (g *Game) Draw(screen *ebiten.Image) {
	screen.Fill(color.RGBA{10, 10, 10, 255})

	sw, sh := 1280.0, 720.0
	blockW, blockH := sw/16.0, sh/9.0

	// --- TITLE SCREEN DRAW ---
	if g.state == StateTitleScreen {
		// 1. Draw Title Background
		titleBg := getImage(AssetsPath + "images/title.png")
		if titleBg != nil {
			// Mapping (1,1) with size (1,1) makes our helper fill the whole screen cleanly
			drawGridImage(screen, titleBg, 1, 1, 1, 1, 1, 1)
		}

		// 2. Draw Title Text (in case the artwork doesn't have it baked in)
		gameTitle := ""
		tw, _ := text.Measure(gameTitle, titleFace, titleFace.Size)
		// Draw drop shadow
		drawText(screen, gameTitle, titleFace, (sw-tw)/2+4, sh*0.2+4, color.RGBA{0, 0, 0, 200})
		// Draw main text
		drawText(screen, gameTitle, titleFace, (sw-tw)/2, sh*0.2, colorPanelLine)

		// 3. Draw "Begin" Button
		btnW, btnH := blockW*4.0, blockH*1.5
		btnX, btnY := (sw-btnW)/2.0, sh*0.75

		mx, my := ebiten.CursorPosition()
		bgCol := colorChoice
		if float64(mx) >= btnX && float64(mx) <= btnX+btnW && float64(my) >= btnY && float64(my) <= btnY+btnH {
			bgCol = colorChoiceHov
		}

		drawUIBox(screen, btnX, btnY, btnW, btnH, bgCol, colorPanelLine)

		btnTxt := "BEGIN"
		bw, bh := text.Measure(btnTxt, nameFace, nameFace.Size)
		drawText(screen, btnTxt, nameFace, btnX+(btnW-bw)/2, btnY+(btnH-bh)/2, colorText)

		return
	}

	// --- VISUAL NOVEL DRAW ---

	// 1. DYNAMIC BACKGROUND
	if g.vn.Background != "" {
		bgImg := getImage(AssetsPath + g.vn.Background)
		// Map the background to an 8x6 grid size inside the 16x9 framework.
		drawGridImage(screen, bgImg, 4.5, 1, 16, 9, 9, 9)
	}

	// 2. DYNAMIC CHARACTER SPRITES
	for _, sprite := range g.vn.ActiveSprites {
		img := getImage(AssetsPath + sprite.FinalLocation)
		drawGridImage(screen, img, sprite.Column, sprite.Row, sprite.WRatio, sprite.HRatio, sprite.WFrameRatio, sprite.HFrameRatio)
	}

	switch g.vn.GetState() {
	case runtime.StateWaiting:
		drawDialogueBox(screen, g.vn.DialogueSpeaker, g.vn.DialogueText)

	case runtime.StateChoice:
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
		drawDialogueBox(screen, "SYSTEM", "End of Script.\n(Press R to return to Title)")
	}
}

// --- HELPER FUNCTIONS ---
func getImage(path string) *ebiten.Image {
	if path == "" {
		return nil
	}
	if img, ok := imageCache[path]; ok {
		return img
	}

	// Go's embed package STRICTLY requires forward slashes for paths.
	// We replace Windows backslashes (\) with forward slashes (/) just in case.
	safePath := strings.ReplaceAll(path, "\\", "/")

	// OPEN USING EMBEDDED FILE SYSTEM INSTEAD OF os.Open!
	file, err := embeddedAssets.Open(safePath)
	if err != nil {
		log.Printf("[HikarinVN Error] Failed to load embedded image: \"%s\"\nError: %v", safePath, err)
		imageCache[path] = createMissingTexture(path)
		return imageCache[path]
	}
	defer file.Close()

	img, _, err := image.Decode(file)
	if err != nil {
		log.Printf("[HikarinVN Error] Failed to decode image: \"%s\"\nError: %v", path, err)
		imageCache[path] = createMissingTexture(path)
		return imageCache[path]
	}

	eImg := ebiten.NewImageFromImage(img)
	imageCache[path] = eImg
	return eImg
}

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

func drawGridImage(screen, img *ebiten.Image, col, row, wRatio, hRatio, frameW, frameH float64) {
	if img == nil {
		return
	}
	sw, sh := 1280.0, 720.0

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

func getChoiceBounds(index, total int, blockW, blockH float64) (x, y, w, h float64) {
	frameW, frameH := 7.0, 0.9
	spacing := 1.2

	totalHeight := float64(total) * spacing
	startRow := (9.0 - totalHeight) / 2.0

	col := 4.5
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

	// NOTE: We REMOVED vn.Start("") from here so it doesn't run the VN until you hit Begin!

	// Create the Game and set state to TitleScreen
	g := &Game{
		vn:    vn,
		state: StateTitleScreen,
	}

	if err := ebiten.RunGame(g); err != nil {
		log.Fatal(err)
	}
}
