package hikarin_runtime

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"regexp"
	"strings"
)

// Choice represents a single option in a choice menu.
type Choice struct {
	Label   string `json:"label"`
	Display string `json:"display"`
}

// SpriteInfo holds the data for a displayed sprite.
type SpriteInfo struct {
	Sprite        string `json:"sprite"`
	Location      string `json:"location"`
	DynLocation   string `json:"dyn_location,omitempty"`
	FinalLocation string `json:"finalLocation,omitempty"`
	Position      string `json:"position"`
}

// VNState indicates what the engine is waiting for.
type VNState string

const (
	StateIdle    VNState = "IDLE"
	StatePlaying VNState = "PLAYING"
	StateWaiting VNState = "WAITING" // dialogue – waiting for advance
	StateChoice  VNState = "CHOICE"  // choices – waiting for selection
	StateEnded   VNState = "ENDED"
)

// OnEventFunc is a callback for various engine events (matches JS hooks).
type OnEventFunc func(eventType string, data interface{})

// VisualNovelRuntime replicates the JavaScript engine.
type VisualNovelRuntime struct {
	script       []map[string]interface{}
	currentIndex int
	state        VNState

	variables   map[string]interface{} // local
	globals     map[string]interface{} // global
	environment map[string]interface{} // isNight, isDay

	// Public display fields
	DialogueSpeaker string
	DialogueText    string
	Choices         []Choice
	Background      string
	Music           string
	ActiveSprites   map[string]SpriteInfo // keyed by sprite ID

	// Callback (optional)
	OnEvent OnEventFunc
}

// NewRuntime creates a new runtime with maps.
func NewRuntime() *VisualNovelRuntime {
	return &VisualNovelRuntime{
		variables:     make(map[string]interface{}),
		globals:       make(map[string]interface{}),
		environment:   map[string]interface{}{"isNight": false, "isDay": true},
		ActiveSprites: make(map[string]SpriteInfo),
	}
}

// LoadScript decodes a JSON byte slice (the FSM) and resets the runtime.
func (r *VisualNovelRuntime) LoadScript(jsonData []byte, savedGlobals, savedVariables map[string]interface{}) error {
	var script []map[string]interface{}
	if err := json.Unmarshal(jsonData, &script); err != nil {
		return err
	}
	r.script = script

	// Restore globals/variables (optional)
	r.globals = make(map[string]interface{})
	for k, v := range savedGlobals {
		r.globals[k] = v
	}
	r.variables = make(map[string]interface{})
	for k, v := range savedVariables {
		r.variables[k] = v
	}

	r.currentIndex = 0
	r.state = StateIdle
	r.ActiveSprites = make(map[string]SpriteInfo)
	return nil
}

// Start begins execution from the given label. If label is empty, starts from
// the first instruction of the script (or the last autosave if present).
func (r *VisualNovelRuntime) Start(startLabel string) {
	if len(r.script) == 0 {
		return
	}
	r.state = StatePlaying

	// Autosave resume
	if startLabel == "" {
		if autosave, ok := r.variables["_autosave"].(string); ok {
			startLabel = autosave
		}
	}
	if startLabel != "" {
		r.jump(startLabel)
	} else {
		r.currentIndex = 0
		r.step()
	}
}

// Advance moves past a dialogue and continues execution.
func (r *VisualNovelRuntime) Advance() {
	if r.state != StateWaiting {
		return
	}
	r.state = StatePlaying
	r.currentIndex++
	r.step()
}

// SelectChoice jumps to the chosen label.
func (r *VisualNovelRuntime) SelectChoice(label string) {
	if r.state != StateChoice {
		return
	}
	r.jump(label)
}

// SetEnvironment updates a key in the environment map (e.g., "isNight").
func (r *VisualNovelRuntime) SetEnvironment(key string, value interface{}) {
	r.environment[key] = value
}

// ------------------------------------------------------------
// Internal stepping logic
// ------------------------------------------------------------
func (r *VisualNovelRuntime) step() {
	if r.currentIndex >= len(r.script) {
		r.finish()
		return
	}

	step := r.script[r.currentIndex]
	stepType, _ := step["type"].(string)
	id, _ := step["id"].(string)
	r.log("STEP", fmt.Sprintf("[ID:%s] TYPE: %s", id, stepType))

	proceed := func() {
		r.currentIndex++
		r.step()
	}

	switch stepType {
	case "conditional", "conditional_global":
		passed := r.checkCondition(step)
		if passed {
			proceed()
		} else {
			if endID, ok := step["end"].(string); ok && endID != "" {
				r.jumpToId(endID)
			} else {
				proceed() // no end, just continue
			}
		}
	case "label",
		"start",
		"meta",
		"command":
		if stepType == "meta" {
			r.processMeta(step)
		}
		proceed()
	case "transition":
		if label, ok := step["label"].(string); ok {
			r.jump(label)
		}
	case "next":
		if label, ok := step["label"].(string); ok {
			r.variables["_autosave"] = label
		}
		proceed()
	case "dialogue":
		r.state = StateWaiting
		speaker := r.parseString(getString(step, "label"))
		content := r.parseString(getString(step, "content"))
		r.DialogueSpeaker = speaker
		r.DialogueText = content
		if r.OnEvent != nil {
			r.OnEvent("say", map[string]string{"speaker": speaker, "text": content})
		}
	case "choice":
		r.state = StateChoice
		choices := r.parseChoices(step)
		r.Choices = choices
		if r.OnEvent != nil {
			r.OnEvent("choice", choices)
		}
	case "show_sprite":
		finalLoc := r.resolveSpriteLocation(step)
		if finalLoc != "" {
			sprite := SpriteInfo{
				Sprite:        getString(step, "sprite"),
				Location:      getString(step, "location"),
				DynLocation:   getString(step, "dyn_location"),
				FinalLocation: finalLoc,
				Position:      getString(step, "position"),
			}
			r.ActiveSprites[sprite.Sprite] = sprite
			if r.OnEvent != nil {
				r.OnEvent("show_sprite", sprite)
			}
		}
		proceed()
	case "remove_sprite":
		sprite, _ := step["sprite"].(string)
		delete(r.ActiveSprites, sprite)
		if r.OnEvent != nil {
			r.OnEvent("remove_sprite", sprite)
		}
		proceed()
	case "modify_background":
		bg := r.parseString(getString(step, "background"))
		r.Background = bg
		if r.OnEvent != nil {
			r.OnEvent("background", bg)
		}
		proceed()
	case "play_music":
		music := r.parseString(getString(step, "music"))
		r.Music = music
		if r.OnEvent != nil {
			r.OnEvent("music", music)
		}
		proceed()
	case "stop_music":
		r.Music = ""
		if r.OnEvent != nil {
			r.OnEvent("music_stop", nil)
		}
		proceed()
	case "modify_variable":
		r.modVar(r.variables, step, "Local")
		proceed()
	case "modify_global":
		r.modVar(r.globals, step, "Global")
		proceed()
	case "finish_dialogue":
		r.finish()
	case "unlock_dialogues":
		r.unlockDialogues(step)
		proceed()
	case "idle_chat":
		r.idleChat()
		// idleChat jumps or proceeds internally
	case "random_dialogue":
		r.randomDialogue(step)
	default:
		r.log("ERR", fmt.Sprintf("Unknown Instruction: %s", stepType))
		proceed()
	}
}

// Helper functions ---------------------------------------------------

func (r *VisualNovelRuntime) finish() {
	r.state = StateEnded
	if r.OnEvent != nil {
		r.OnEvent("finish", nil)
	}
}

func (r *VisualNovelRuntime) jumpToId(targetId string) {
	for i, s := range r.script {
		if sid, _ := s["id"].(string); sid == targetId {
			r.currentIndex = i
			r.state = StatePlaying
			r.step()
			return
		}
	}
	r.log("ERR", fmt.Sprintf("CRITICAL: Could not find step with ID: %s", targetId))
}

func (r *VisualNovelRuntime) jump(labelName string) {
	for i, s := range r.script {
		if stype, _ := s["type"].(string); stype == "label" {
			if slabel, _ := s["label"].(string); slabel == labelName {
				r.currentIndex = i
				r.state = StatePlaying
				r.step()
				return
			}
		}
	}
	r.log("ERR", fmt.Sprintf("CRITICAL: Label '%s' not found!", labelName))
}

func (r *VisualNovelRuntime) processMeta(step map[string]interface{}) {
	action, _ := step["action"].(string)
	switch action {
	case "create_var":
		if varname, ok := step["var"].(string); ok {
			r.variables[varname] = step["init"]
		}
	case "create_global":
		if varname, ok := step["var"].(string); ok {
			if _, exists := r.globals[varname]; !exists {
				r.globals[varname] = step["init"]
			}
		}
	}
}

func (r *VisualNovelRuntime) modVar(scope map[string]interface{}, step map[string]interface{}, label string) {
	key, _ := step["var"].(string)
	if key == "" {
		return
	}
	if _, exists := scope[key]; !exists {
		scope[key] = 0
	}

	action, _ := step["action"].(string)
	val := step["value"]

	// Ensure numeric if modifying
	current, _ := toFloat(scope[key])
	modValue, _ := toFloat(val)

	switch action {
	case "modify_var":
		scope[key] = val
	case "increment_var":
		scope[key] = current + modValue
	case "subtract_var":
		scope[key] = current - modValue
	}
	r.log("VAR", fmt.Sprintf("[%s] %s = %v", label, key, scope[key]))
}

func (r *VisualNovelRuntime) checkCondition(step map[string]interface{}) bool {
	varName, _ := step["var"].(string)
	targetValue := step["value"]
	condition, _ := step["condition"].(string)

	var val interface{}
	if _, ok := r.environment[varName]; ok {
		val = r.environment[varName]
	} else if stepType, _ := step["type"].(string); stepType == "conditional_global" {
		val = r.globals[varName]
	} else {
		val = r.variables[varName]
	}
	if val == nil {
		val = 0
	}

	valFloat, _ := toFloat(val)
	targetFloat, _ := toFloat(targetValue)

	switch condition {
	case "equal":
		return compare(val, targetValue) == 0
	case "not_equal":
		return compare(val, targetValue) != 0
	case "greater_than":
		return valFloat > targetFloat
	case "less_than":
		return valFloat < targetFloat
	default:
		return false
	}
}

func (r *VisualNovelRuntime) parseString(str string) string {
	re := regexp.MustCompile(`<([^>]+)>`)
	return re.ReplaceAllStringFunc(str, func(match string) string {
		key := match[1 : len(match)-1]
		if val, ok := r.globals[key]; ok {
			return fmt.Sprintf("%v", val)
		}
		if val, ok := r.variables[key]; ok {
			return fmt.Sprintf("%v", val)
		}
		return match
	})
}

func (r *VisualNovelRuntime) parseChoices(step map[string]interface{}) []Choice {
	choicesRaw, ok := step["choice"].([]interface{})
	if !ok {
		return nil
	}
	var out []Choice
	for _, c := range choicesRaw {
		m, ok := c.(map[string]interface{})
		if !ok {
			continue
		}
		label, _ := m["label"].(string)
		display, _ := m["display"].(string)
		out = append(out, Choice{
			Label:   label,
			Display: r.parseString(display),
		})
	}
	return out
}

func (r *VisualNovelRuntime) resolveSpriteLocation(step map[string]interface{}) string {
	// Try dynamic location first
	dynLoc, _ := step["dyn_location"].(string)
	if dynLoc != "" {
		parsed := r.parseString(dynLoc)
		if !strings.Contains(parsed, "<") {
			return parsed
		}
	}
	// Fallback to static location
	loc, _ := step["location"].(string)
	return r.parseString(loc)
}

func (r *VisualNovelRuntime) unlockDialogues(step map[string]interface{}) {
	events, ok := step["events"].([]interface{})
	if !ok {
		return
	}
	unlocked, ok := r.variables["_unlocked_dialogues"].([]interface{})
	if !ok {
		unlocked = make([]interface{}, 0)
	}
	for _, ev := range events {
		label, ok := ev.(string)
		if !ok {
			continue
		}
		found := false
		for _, existing := range unlocked {
			if s, ok := existing.(string); ok && s == label {
				found = true
				break
			}
		}
		if !found {
			unlocked = append(unlocked, label)
		}
	}
	r.variables["_unlocked_dialogues"] = unlocked
}

func (r *VisualNovelRuntime) idleChat() {
	available, ok := r.variables["_unlocked_dialogues"].([]interface{})
	if !ok || len(available) == 0 {
		// no chats, just proceed
		r.currentIndex++
		r.step()
		return
	}
	idx := rand.Intn(len(available))
	label, _ := available[idx].(string)
	if label != "" {
		r.jump(label)
	}
}

func (r *VisualNovelRuntime) randomDialogue(step map[string]interface{}) {
	events, ok := step["events"].([]interface{})
	if !ok || len(events) == 0 {
		r.currentIndex++
		r.step()
		return
	}
	idx := rand.Intn(len(events))
	label, _ := events[idx].(string)
	if label != "" {
		r.jump(label)
	}
}

// ------------------------------------------------------------
// Utility functions
// ------------------------------------------------------------
func getString(m map[string]interface{}, key string) string {
	s, _ := m[key].(string)
	return s
}

func toFloat(v interface{}) (float64, bool) {
	switch val := v.(type) {
	case float64:
		return val, true
	case int:
		return float64(val), true
	case int64:
		return float64(val), true
	case json.Number:
		f, err := val.Float64()
		return f, err == nil
	default:
		return 0, false
	}
}

func compare(a, b interface{}) int {
	// tries to compare as float, then as string
	f1, ok1 := toFloat(a)
	f2, ok2 := toFloat(b)
	if ok1 && ok2 {
		switch {
		case f1 < f2:
			return -1
		case f1 > f2:
			return 1
		default:
			return 0
		}
	}
	s1 := fmt.Sprintf("%v", a)
	s2 := fmt.Sprintf("%v", b)
	return strings.Compare(s1, s2)
}

func (r *VisualNovelRuntime) log(category, msg string) {
	// For debug; can be replaced with a callback
	if r.OnEvent != nil {
		r.OnEvent("log", map[string]string{"category": category, "message": msg})
	}
}

// Getters for state (optional)
func (r *VisualNovelRuntime) GetState() VNState     { return r.state }
func (r *VisualNovelRuntime) GetBackground() string { return r.Background }
func (r *VisualNovelRuntime) GetMusic() string      { return r.Music }
func (r *VisualNovelRuntime) GetChoices() []Choice  { return r.Choices }
