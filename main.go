package main

import (
	"log"

	runtime "github.com/Iteranya/ToxicEbitenGamejam2026/hikarin_runtime"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
)

//go:embed story.json
var storyJSON []byte

type Game struct {
	vn *runtime.VisualNovelRuntime
}

func (g *Game) Update() error {
	switch g.vn.GetState() {
	case runtime.StateWaiting:
		if inpututil.IsKeyJustPressed(ebiten.KeySpace) || inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
			g.vn.Advance()
		}
	case runtime.StateChoice:
		// For now, skip choices—you'll wire them later
	case runtime.StateEnded:
		// Do nothing (or exit)
	}
	return nil
}

func (g *Game) Draw(screen *ebiten.Image) {
	ebitenutil.DebugPrint(screen, g.vn.DialogueSpeaker+": "+g.vn.DialogueText)
}

func (g *Game) Layout(outsideWidth, outsideHeight int) (int, int) {
	return 800, 600
}

func main() {
	vn := runtime.NewRuntime()
	if err := vn.LoadScript(storyJSON, nil, nil); err != nil {
		log.Fatal(err)
	}
	vn.Start("")

	g := &Game{vn: vn}

	ebiten.SetWindowSize(800, 600)
	ebiten.SetWindowTitle("Blood Slave Rapture")
	if err := ebiten.RunGame(g); err != nil {
		log.Fatal(err)
	}
}
