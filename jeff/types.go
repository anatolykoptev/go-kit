package jeff

import "strconv"

// Question is one typed question in a System One request. Criteria is
// polymorphic by Type — use the Noul/Choice/Score constructors rather
// than populating the struct directly.
type Question struct {
	Type         string `json:"type"`
	Instructions any    `json:"instructions,omitempty"`
	Criteria     any    `json:"criteria,omitempty"`
}

// NoulQuestion builds a yes/no question. The service returns Answer.Noul —
// the probability of "yes".
func NoulQuestion(instructions string) Question {
	return Question{Type: "noul", Instructions: instructions}
}

// NoulCriteria is the optional two-sided criteria block for a noul
// question — explicit true/false descriptions sharpen the verdict.
type NoulCriteria struct {
	True  any `json:"true,omitempty"`
	False any `json:"false,omitempty"`
}

// ChoiceQuestion builds a pick-one question. options maps an option name
// to its description (any JSON value or nil).
func ChoiceQuestion(instructions string, options map[string]any) Question {
	return Question{Type: "choice", Instructions: instructions, Criteria: options}
}

// ScoreQuestion builds an ordered-scale question. levels is the scale from
// lowest to highest. The service returns Answer.Score as the
// probability-weighted mean level (continuous, 0..len(levels)-1), not the
// chosen level — use Answer.Level for that.
func ScoreQuestion(instructions string, levels []any) Question {
	return Question{Type: "score", Instructions: instructions, Criteria: levels}
}

// Request is the /v1/systemone request body.
type Request struct {
	State          any                 `json:"state"`
	Model          string              `json:"model,omitempty"`
	SelectedModels []string            `json:"selectedModels,omitempty"`
	Questions      map[string]Question `json:"questions"`
}

// Answer is one entry of Response.Answers. Which fields are populated
// depends on Type: "noul" → Noul; "choice" → Choice/Confidence/
// Probabilities; "score" → Score/Confidence/Legend/Probabilities.
//
// For "score", Score is the expected level Σ i·p(i) over the uncalibrated
// distribution, so it is fractional and can sit between levels that were
// never likely (p(0)=0.55, p(2)=0.45 gives 0.9 → "level 1"). Probabilities
// is keyed by the level index as a string ("0", "1", …); Level returns
// its argmax.
type Answer struct {
	Type          string             `json:"type"`
	Noul          float64            `json:"noul,omitempty"`
	Choice        string             `json:"choice,omitempty"`
	Confidence    float64            `json:"confidence,omitempty"`
	Probabilities map[string]float64 `json:"probabilities,omitempty"`
	Score         float64            `json:"score,omitempty"`
	Legend        map[string]any     `json:"legend,omitempty"`
}

// Usage reports token accounting for one request.
type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// Response is the /v1/systemone response body.
type Response struct {
	Model   string            `json:"model"`
	Answers map[string]Answer `json:"answers"`
	Usage   Usage             `json:"usage"`
}

// Level returns the most probable level of a "score" answer — the argmax
// of Probabilities, whose keys are level indices. ok is false when the
// answer carries no index-keyed probabilities.
func (a Answer) Level() (level int, ok bool) {
	best := -1.0
	for k, p := range a.Probabilities {
		i, err := strconv.Atoi(k)
		if err != nil || i < 0 {
			continue
		}
		if p > best || (p == best && i < level) {
			level, best, ok = i, p, true
		}
	}
	return level, ok
}
