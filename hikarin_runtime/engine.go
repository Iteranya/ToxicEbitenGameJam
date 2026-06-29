package hikarin_runtime

import (
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"regexp"
	"strings"
	"time"
)

// Choice represents a single option in a choice menu.
type Choice struct {
	Label   string `json:"label"`
	Display string `json:"display"`
}

// SpriteInfo holds the data for a displayed sprite.
type SpriteInfo struct {
	Sprite        string  `json:"sprite"`
	Location      string  `json:"location"`
	DynLocation   string  `json:"dyn_location,omitempty"`
	FinalLocation string  `json:"finalLocation,omitempty"`
	Position      string  `json:"position"`
	Column        float64 `json:"column"`
	Row           float64 `json:"row"`
	WRatio        float64 `json:"wRatio"`
	HRatio        float64 `json:"hRatio"`
	WFrameRatio   float64 `json:"wFrameRatio"`
	HFrameRatio   float64 `json:"hFrameRatio"`
}

// LogEntry represents a structured log event, mimicking the JS payload.
type LogEntry struct {
	Category  string        `json:"category"`
	Message   string        `json:"message"`
	Args      []interface{} `json:"args"`
	Timestamp time.Time     `json:"timestamp"`
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
// Event types: "say", "choice", "show_sprite", "remove_sprite", "background",
// "music", "music_stop", "autosave", "finish", "log"
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

// LoadScript decodes a JSON byte slice (the FSM) and restores states.
func (r *VisualNovelRuntime) LoadScript(jsonData []byte, savedGlobals, savedVariables map[string]interface{}) error {
	var script []map[string]interface{}
	if err := json.Unmarshal(jsonData, &script); err != nil {
		return err
	}
	r.script = script

	// Restore globals
	r.globals = make(map[string]interface{})
	for k, v := range savedGlobals {
		r.globals[k] = v
	}

	// Restore locals (variables) instead of wiping them entirely
	r.variables = make(map[string]interface{})
	for k, v := range savedVariables {
		r.variables[k] = v
	}

	r.currentIndex = 0
	r.state = StateIdle
	r.ActiveSprites = make(map[string]SpriteInfo)

	r.log("FLOW", fmt.Sprintf("Script Loaded. Total Steps: %d", len(r.script)))
	return nil
}

// Start begins execution. If startLabel is empty, it attempts to resume from _autosave or index 0.
func (r *VisualNovelRuntime) Start(startLabel string) {
	if len(r.script) == 0 {
		return
	}
	r.state = StatePlaying

	// 1. If no specific label is forced, check if we have an autosave variable
	if startLabel == "" {
		if autosave, ok := r.variables["_autosave"].(string); ok && autosave != "" {
			r.log("FLOW", fmt.Sprintf("Found '_autosave' variable. Resuming at label: '%s'", autosave))
			startLabel = autosave
		}
	}

	// 2. Execute Jump or Start from 0
	if startLabel != "" {
		r.log("FLOW", fmt.Sprintf("Starting execution at label: %s", startLabel))
		r.jump(startLabel)
	} else {
		r.currentIndex = 0
		r.step()
	}
}

// Stop safely sets the state back to idle
func (r *VisualNovelRuntime) Stop() {
	r.state = StateIdle
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
func (r *VisualNovelRuntime) SelectChoice(labelToJumpTo string) {
	if r.state != StateChoice {
		return
	}
	r.log("FLOW", fmt.Sprintf("Player selected choice -> Jumping to '%s'", labelToJumpTo))
	r.jump(labelToJumpTo)
}

// SetEnvironment updates a key in the environment map (e.g., "isNight").
func (r *VisualNovelRuntime) SetEnvironment(key string, value interface{}) {
	if _, exists := r.environment[key]; exists {
		r.log("FLOW", fmt.Sprintf("Environment updated: %s set to %v", key, value))
		r.environment[key] = value
	} else {
		r.log("WARN", fmt.Sprintf("Attempted to set unknown environment key: '%s'", key))
	}
}

// ------------------------------------------------------------
// Public API for Variable CRUD (Matches JS)
// ------------------------------------------------------------

func (r *VisualNovelRuntime) SetVariable(key string, value interface{}) {
	r.log("VAR", fmt.Sprintf("External set LOCAL variable: '%s' = %v", key, value))
	r.variables[key] = value
}

func (r *VisualNovelRuntime) SetGlobal(key string, value interface{}) {
	r.log("VAR", fmt.Sprintf("External set GLOBAL variable: '%s' = %v", key, value))
	r.globals[key] = value
}

func (r *VisualNovelRuntime) GetVariable(key string) interface{} {
	return r.variables[key]
}

func (r *VisualNovelRuntime) GetGlobal(key string) interface{} {
	return r.globals[key]
}

func (r *VisualNovelRuntime) DeleteVariable(key string) {
	if _, exists := r.variables[key]; exists {
		r.log("VAR", fmt.Sprintf("External delete LOCAL variable: '%s'", key))
		delete(r.variables, key)
	}
}

func (r *VisualNovelRuntime) DeleteGlobal(key string) {
	if _, exists := r.globals[key]; exists {
		r.log("VAR", fmt.Sprintf("External delete GLOBAL variable: '%s'", key))
		delete(r.globals, key)
	}
}

// ------------------------------------------------------------
// Internal Stepping Logic
// ------------------------------------------------------------

func (r *VisualNovelRuntime) step() {
	if r.currentIndex >= len(r.script) {
		r.finish()
		return
	}

	step := r.script[r.currentIndex]
	stepType, _ := step["type"].(string)
	id, _ := step["id"].(string)

	proceed := func() {
		r.currentIndex++
		r.step()
	}

	r.log("STEP", fmt.Sprintf("[ID:%s] TYPE: %s", id, stepType))

	switch stepType {
	case "conditional", "conditional_global":
		passed := r.checkCondition(step)
		if passed {
			r.log("COND", "Condition PASSED. Continuing flow.")
			proceed()
		} else {
			if endID, ok := step["end"].(string); ok && endID != "" {
				r.log("COND", fmt.Sprintf("Condition FAILED. Skipping to ID: %s", endID))
				r.jumpToId(endID)
			} else {
				r.log("ERR", "Condition Failed but no 'end' ID provided!")
				proceed()
			}
		}

	case "label", "start", "meta", "command":
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
			r.variables["_autosave"] = label // Save it to vars
			if r.OnEvent != nil {
				r.OnEvent("autosave", label)
			}
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
		finalLoc := ""

		// 1. Try dynamic location first
		dynLoc, _ := step["dyn_location"].(string)
		if dynLoc != "" {
			parsed := r.parseString(dynLoc)
			// Ensure it actually resolved (doesn't contain `<tag>`)
			if !strings.Contains(parsed, "<") && !strings.Contains(parsed, ">") {
				finalLoc = parsed
			} else {
				r.log("WARN", fmt.Sprintf("Dynamic sprite '%s' failed to resolve vars. Falling back.", dynLoc))
			}
		}

		// 2. Fallback to static location
		if finalLoc == "" {
			finalLoc = r.parseString(getString(step, "location"))
		}

		if finalLoc != "" {
			sprite := SpriteInfo{
				Sprite:        getString(step, "sprite"),
				Location:      getString(step, "location"),
				DynLocation:   dynLoc,
				FinalLocation: finalLoc,
				Position:      getString(step, "position"),
				Column:        getFloat(step, "column"),
				Row:           getFloat(step, "row"),
				WRatio:        getFloat(step, "wRatio"),
				HRatio:        getFloat(step, "hRatio"),
				WFrameRatio:   getFloat(step, "wFrameRatio"),
				HFrameRatio:   getFloat(step, "hFrameRatio"),
			}
			r.ActiveSprites[sprite.Sprite] = sprite
			if r.OnEvent != nil {
				r.OnEvent("show_sprite", sprite)
			}
		}
		proceed()

	case "remove_sprite":
		sprite := getString(step, "sprite")
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

	case "unlock_dialogues":
		r.unlockDialogues(step)
		proceed()

	case "idle_chat":
		r.idleChat()

	case "random_dialogue":
		r.randomDialogue(step)

	case "modify_variable":
		r.modVar(r.variables, step, "Local")
		proceed()

	case "modify_global":
		r.modVar(r.globals, step, "Global")
		proceed()

	case "finish_dialogue":
		r.finish()

	default:
		r.log("ERR", fmt.Sprintf("Unknown Instruction: %s", stepType))
		proceed()
	}
}

// ------------------------------------------------------------
// Logic Helpers
// ------------------------------------------------------------

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
				r.log("FLOW", fmt.Sprintf("Jump Successful. Moving Index %d -> %d", r.currentIndex, i))
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
	if action == "create_var" {
		if varname, ok := step["var"].(string); ok {
			r.variables[varname] = step["init"]
		}
	} else if action == "create_global" {
		if varname, ok := step["var"].(string); ok {
			if _, exists := r.globals[varname]; !exists {
				r.globals[varname] = step["init"]
			}
		}
	}
}

func (r *VisualNovelRuntime) modVar(scope map[string]interface{}, step map[string]interface{}, label string) {
	key := getString(step, "var")
	if key == "" {
		return
	}
	if _, exists := scope[key]; !exists {
		scope[key] = 0.0
	}

	action, _ := step["action"].(string)
	val := step["value"]

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
}

func (r *VisualNovelRuntime) checkCondition(step map[string]interface{}) bool {
	varName := getString(step, "var")
	targetValue := step["value"]
	condition := getString(step, "condition")
	stepType := getString(step, "type")

	var val interface{}
	valFound := false

	// Check Environment First
	if envVal, exists := r.environment[varName]; exists {
		r.log("COND", fmt.Sprintf("Checking Environment property: '%s'", varName))
		val = envVal
		valFound = true
	} else if stepType == "conditional_global" {
		r.log("COND", fmt.Sprintf("Checking Global variable: '%s'", varName))
		if gVal, exists := r.globals[varName]; exists {
			val = gVal
			valFound = true
		}
	} else {
		r.log("COND", fmt.Sprintf("Checking Local variable: '%s'", varName))
		if lVal, exists := r.variables[varName]; exists {
			val = lVal
			valFound = true
		}
	}

	if !valFound {
		val = 0
	}

	result := false
	valFloat, vOk := toFloat(val)
	targetFloat, tOk := toFloat(targetValue)

	if condition == "equal" {
		result = compare(val, targetValue) == 0
	} else if condition == "not_equal" {
		result = compare(val, targetValue) != 0
	} else if vOk && tOk {
		if condition == "greater_than" {
			result = valFloat > targetFloat
		} else if condition == "less_than" {
			result = valFloat < targetFloat
		}
	}

	r.log("COND", fmt.Sprintf("Check '%s' (%v) %s '%v'? Result: %v", varName, val, condition, targetValue, result))
	return result
}

func (r *VisualNovelRuntime) parseString(str string) string {
	if str == "" {
		return ""
	}
	re := regexp.MustCompile(`<([^>]+)>`)
	return re.ReplaceAllStringFunc(str, func(match string) string {
		key := match[1 : len(match)-1]
		if val, ok := r.globals[key]; ok {
			return fmt.Sprintf("%v", val)
		}
		if val, ok := r.variables[key]; ok {
			return fmt.Sprintf("%v", val)
		}
		return match // Leave as `<key>` if unresolved
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
		out = append(out, Choice{
			Label:   getString(m, "label"),
			Display: r.parseString(getString(m, "display")),
		})
	}
	return out
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

	var newlyAdded []string
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
			newlyAdded = append(newlyAdded, label)
		}
	}

	if len(newlyAdded) > 0 {
		r.variables["_unlocked_dialogues"] = unlocked
		r.log("VAR", fmt.Sprintf("Unlocked dialogues: %s", strings.Join(newlyAdded, ", ")))
	}
}

func (r *VisualNovelRuntime) idleChat() {
	available, ok := r.variables["_unlocked_dialogues"].([]interface{})
	if ok && len(available) > 0 {
		idx := rand.Intn(len(available))
		if label, ok := available[idx].(string); ok && label != "" {
			r.log("FLOW", fmt.Sprintf("Performing idle chat. Randomly selected: '%s'", label))
			r.jump(label)
			return
		}
	}
	r.log("WARN", "Idle chat triggered, but '_unlocked_dialogues' is empty or not found. Skipping.")
	r.currentIndex++
	r.step()
}

func (r *VisualNovelRuntime) randomDialogue(step map[string]interface{}) {
	events, ok := step["events"].([]interface{})
	if ok && len(events) > 0 {
		idx := rand.Intn(len(events))
		if label, ok := events[idx].(string); ok && label != "" {
			r.log("FLOW", fmt.Sprintf("Performing random dialogue. Randomly selected: '%s'", label))
			r.jump(label)
			return
		}
	}
	r.log("WARN", "Random dialogue triggered, but 'events' array is empty or not found. Skipping.")
	r.currentIndex++
	r.step()
}

// ------------------------------------------------------------
// Logger & Data Utilities
// ------------------------------------------------------------

func (r *VisualNovelRuntime) log(category, msg string, args ...interface{}) {
	// Standard output fallback
	log.Printf("[%s] %s %v\n", category, msg, args)

	// Emit structured hook event matching JS behavior
	if r.OnEvent != nil {
		entry := LogEntry{
			Category:  category,
			Message:   msg,
			Args:      args,
			Timestamp: time.Now(),
		}
		r.OnEvent("log", entry)
	}
}

func getString(m map[string]interface{}, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func getFloat(m map[string]interface{}, key string) float64 {
	if f, ok := toFloat(m[key]); ok {
		return f
	}
	return 1.0 // Safe default to prevent division by zero in UI scaling
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
	f1, ok1 := toFloat(a)
	f2, ok2 := toFloat(b)
	if ok1 && ok2 {
		if f1 < f2 {
			return -1
		}
		if f1 > f2 {
			return 1
		}
		return 0
	}
	return strings.Compare(fmt.Sprintf("%v", a), fmt.Sprintf("%v", b))
}

// Getters for state
func (r *VisualNovelRuntime) GetState() VNState     { return r.state }
func (r *VisualNovelRuntime) GetBackground() string { return r.Background }
func (r *VisualNovelRuntime) GetMusic() string      { return r.Music }
func (r *VisualNovelRuntime) GetChoices() []Choice  { return r.Choices }
