package dto

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/relaykit/relayconvert/kitutil"
	"github.com/QuantumNous/new-api/relaykit/types"
)

// Decisions keeps the two native protocols distinct. RawBody is forwarded only
// after validation, preserving optional zero values and provider extensions.
type DecisionsRequest interface {
	Request
	DecisionsFormat() types.RelayFormat
	DecisionsBody() json.RawMessage
}

type OpenAIDecisionsRequest struct {
	Model            string                   `json:"model"`
	Input            json.RawMessage          `json:"input"`
	Questions        []OpenAIDecisionQuestion `json:"questions"`
	SafetyIdentifier *string                  `json:"safety_identifier,omitempty"`
	Stream           *bool                    `json:"stream,omitempty"`
	RawBody          json.RawMessage          `json:"-"`
}

type OpenAIDecisionQuestion struct {
	Type         string                 `json:"type"`
	Name         *string                `json:"name,omitempty"`
	Instructions string                 `json:"instructions"`
	Choices      []OpenAIDecisionChoice `json:"choices,omitempty"`
	Levels       []OpenAIDecisionLevel  `json:"levels,omitempty"`
}

type OpenAIDecisionChoice struct {
	Value       any     `json:"value"` // string or bool, including false
	Description *string `json:"description,omitempty"`
}

type OpenAIDecisionLevel struct {
	Label       string  `json:"label"`
	Description *string `json:"description,omitempty"`
}

type JEVDecisionsRequest struct {
	Model     string                         `json:"model"`
	State     json.RawMessage                `json:"state"`
	Questions map[string]JEVDecisionQuestion `json:"questions"`
	Stream    *bool                          `json:"stream,omitempty"`
	RawBody   json.RawMessage                `json:"-"`
}

type JEVDecisionQuestion struct {
	Type         string          `json:"type"`
	Instructions json.RawMessage `json:"instructions"`
	Criteria     json.RawMessage `json:"criteria,omitempty"`
}

func (r *OpenAIDecisionsRequest) GetTokenCountMeta() *types.TokenCountMeta {
	return &types.TokenCountMeta{CombineText: string(r.RawBody), TokenType: types.TokenTypeTokenizer}
}
func (r *JEVDecisionsRequest) GetTokenCountMeta() *types.TokenCountMeta {
	return &types.TokenCountMeta{CombineText: string(r.RawBody), TokenType: types.TokenTypeTokenizer}
}
func (r *OpenAIDecisionsRequest) IsStream(*http.Request) bool { return false }
func (r *JEVDecisionsRequest) IsStream(*http.Request) bool    { return false }
func (r *OpenAIDecisionsRequest) SetModelName(name string) {
	if name != "" {
		r.Model = name
	}
}
func (r *JEVDecisionsRequest) SetModelName(name string) {
	if name != "" {
		r.Model = name
	}
}
func (r *OpenAIDecisionsRequest) DecisionsFormat() types.RelayFormat {
	return types.RelayFormatOpenAIDecisions
}
func (r *JEVDecisionsRequest) DecisionsFormat() types.RelayFormat {
	return types.RelayFormatJEVDecisions
}
func (r *OpenAIDecisionsRequest) DecisionsBody() json.RawMessage { return r.RawBody }
func (r *JEVDecisionsRequest) DecisionsBody() json.RawMessage    { return r.RawBody }

// ParseDecisionsRequest is also used after parameter overrides: an override
// cannot enable streaming or change the native protocol after channel selection.
func ParseDecisionsRequest(body []byte) (DecisionsRequest, error) {
	var fields map[string]json.RawMessage
	if err := kitutil.Unmarshal(body, &fields); err != nil {
		return nil, err
	}
	if fields == nil {
		return nil, errors.New("decisions request must be an object")
	}
	var model string
	if err := kitutil.Unmarshal(fields["model"], &model); err != nil || strings.TrimSpace(model) == "" {
		return nil, errors.New("model is required")
	}
	if raw, exists := fields["stream"]; exists {
		var stream *bool
		if err := kitutil.Unmarshal(raw, &stream); err != nil || stream != nil && *stream {
			return nil, errors.New("decisions does not support streaming")
		}
	}
	_, hasInput := fields["input"]
	_, hasState := fields["state"]
	if hasInput == hasState {
		return nil, errors.New("provide exactly one of input (OpenAI) or state (JEV)")
	}
	if hasInput {
		request := &OpenAIDecisionsRequest{}
		if err := kitutil.Unmarshal(body, request); err != nil {
			return nil, err
		}
		if err := validateOpenAIDecisions(fields, request); err != nil {
			return nil, err
		}
		request.RawBody = append(json.RawMessage(nil), body...)
		return request, nil
	}
	request := &JEVDecisionsRequest{}
	if err := kitutil.Unmarshal(body, request); err != nil {
		return nil, err
	}
	if !structuredDecisionValue(request.State, false) {
		return nil, errors.New("state must be a string, object or array")
	}
	if len(request.Questions) == 0 {
		return nil, errors.New("questions must be a non-empty object for JEV")
	}
	for name, question := range request.Questions {
		if !structuredDecisionValue(question.Instructions, false) {
			return nil, fmt.Errorf("question %q instructions must be a string, object or array", name)
		}
		switch question.Type {
		case "noul", "choice":
			if question.Type == "noul" && len(question.Criteria) == 0 {
				continue
			}
			var criteria map[string]json.RawMessage
			if err := kitutil.Unmarshal(question.Criteria, &criteria); err != nil || criteria == nil {
				return nil, fmt.Errorf("question %q criteria must be an object", name)
			}
			if question.Type == "choice" && (len(criteria) < 1 || len(criteria) > 255) {
				return nil, errors.New("choice requires 1 to 255 criteria")
			}
			for key, value := range criteria {
				if question.Type == "noul" && key != "true" && key != "false" {
					return nil, errors.New("noul criteria keys must be true or false")
				}
				if !structuredDecisionValue(value, question.Type == "choice") {
					return nil, errors.New("invalid JEV criterion")
				}
			}
		case "score":
			var criteria []json.RawMessage
			if err := kitutil.Unmarshal(question.Criteria, &criteria); err != nil || len(criteria) < 2 || len(criteria) > 10 {
				return nil, errors.New("score requires 2 to 10 criteria")
			}
			for _, value := range criteria {
				if !structuredDecisionValue(value, false) {
					return nil, errors.New("invalid score criterion")
				}
			}
		default:
			return nil, fmt.Errorf("unsupported JEV question type %q", question.Type)
		}
	}
	request.RawBody = append(json.RawMessage(nil), body...)
	return request, nil
}

func structuredDecisionValue(raw json.RawMessage, allowNull bool) bool {
	switch kitutil.GetJsonType(raw) {
	case "string", "object", "array":
		return true
	case "null":
		return allowNull
	default:
		return false
	}
}

func validateOpenAIDecisions(fields map[string]json.RawMessage, request *OpenAIDecisionsRequest) error {
	if request.SafetyIdentifier != nil && utf8.RuneCountInString(*request.SafetyIdentifier) > 128 {
		return errors.New("safety_identifier must not exceed 128 characters")
	}
	switch kitutil.GetJsonType(request.Input) {
	case "string":
	case "array":
		var messages []struct {
			Role    string          `json:"role"`
			Type    *string         `json:"type"`
			Content json.RawMessage `json:"content"`
		}
		if err := kitutil.Unmarshal(request.Input, &messages); err != nil || len(messages) == 0 {
			return errors.New("input must contain user messages")
		}
		images := 0
		for _, message := range messages {
			if message.Role != "user" || message.Type != nil && *message.Type != "message" {
				return errors.New("decisions accepts only user messages")
			}
			if kitutil.GetJsonType(message.Content) == "string" {
				continue
			}
			var parts []map[string]json.RawMessage
			if err := kitutil.Unmarshal(message.Content, &parts); err != nil || len(parts) == 0 {
				return errors.New("content must be text or input parts")
			}
			for _, part := range parts {
				var kind string
				if err := kitutil.Unmarshal(part["type"], &kind); err != nil {
					return errors.New("input part type is required")
				}
				switch kind {
				case "input_text":
					var text *string
					if err := kitutil.Unmarshal(part["text"], &text); err != nil || text == nil {
						return errors.New("input_text requires text")
					}
				case "input_image":
					images++
					var imageURL string
					if err := kitutil.Unmarshal(part["image_url"], &imageURL); err != nil {
						return errors.New("input_image requires image_url")
					}
					prefix, encoded, ok := strings.Cut(imageURL, ",")
					if !ok || !strings.HasPrefix(prefix, "data:image/") || !strings.HasSuffix(prefix, ";base64") || encoded == "" {
						return errors.New("images must be inline base64 data URLs")
					}
					if _, err := base64.StdEncoding.DecodeString(encoded); err != nil {
						return errors.New("invalid base64 image")
					}
					if raw, exists := part["detail"]; exists {
						var detail *string
						if err := kitutil.Unmarshal(raw, &detail); err != nil {
							return errors.New("invalid image detail")
						}
						if detail != nil && *detail != "low" && *detail != "high" && *detail != "auto" && *detail != "original" {
							return errors.New("invalid image detail")
						}
					}
				default:
					return errors.New("unsupported decisions input part")
				}
			}
		}
		if images > 128 {
			return errors.New("decisions accepts at most 128 images")
		}
	default:
		return errors.New("input must be text or user messages")
	}
	if len(request.Questions) == 0 {
		return errors.New("questions must be a non-empty array for OpenAI")
	}
	var rawQuestions []map[string]json.RawMessage
	if err := kitutil.Unmarshal(fields["questions"], &rawQuestions); err != nil {
		return err
	}
	for i, question := range request.Questions {
		var instructions *string
		if err := kitutil.Unmarshal(rawQuestions[i]["instructions"], &instructions); err != nil || instructions == nil {
			return errors.New("question instructions is required")
		}
		switch question.Type {
		case "predicate":
		case "choice":
			if len(question.Choices) < 2 || len(question.Choices) > 255 {
				return errors.New("choice requires 2 to 255 choices")
			}
			values := make(map[any]bool, len(question.Choices))
			for _, choice := range question.Choices {
				switch choice.Value.(type) {
				case string, bool:
				default:
					return errors.New("choice value must be a string or boolean")
				}
				if values[choice.Value] {
					return errors.New("choice values must be distinct")
				}
				values[choice.Value] = true
			}
		case "score":
			if len(question.Levels) < 2 {
				return errors.New("score requires at least two levels")
			}
			var levels []map[string]json.RawMessage
			if err := kitutil.Unmarshal(rawQuestions[i]["levels"], &levels); err != nil {
				return err
			}
			for _, level := range levels {
				var label *string
				if err := kitutil.Unmarshal(level["label"], &label); err != nil || label == nil {
					return errors.New("score level label is required")
				}
			}
		default:
			return fmt.Errorf("unsupported OpenAI question type %q", question.Type)
		}
	}
	return nil
}

type DecisionsUsage struct {
	InputTokens  *int64 `json:"input_tokens"`
	OutputTokens *int64 `json:"output_tokens"`
	TotalTokens  *int64 `json:"total_tokens"`
	InputDetails *struct {
		CachedTokens     *int64 `json:"cached_tokens"`
		CacheWriteTokens *int64 `json:"cache_write_tokens"`
	} `json:"input_tokens_details"`
	OutputDetails *struct {
		ReasoningTokens *int64 `json:"reasoning_tokens"`
	} `json:"output_tokens_details"`
}

// ValidateDecisionsResponse validates the full response before anything is
// written to the client or settled. Unknown fields remain in the original body.
func ValidateDecisionsResponse(request DecisionsRequest, body []byte) (*Usage, error) {
	var response struct {
		Model   string          `json:"model"`
		Answers json.RawMessage `json:"answers"`
		Usage   *DecisionsUsage `json:"usage"`
		Error   json.RawMessage `json:"error"`
	}
	if err := kitutil.Unmarshal(body, &response); err != nil {
		return nil, err
	}
	if response.Model == "" || len(response.Error) > 0 && string(response.Error) != "null" {
		return nil, errors.New("invalid decisions response")
	}
	u := response.Usage
	if u == nil || u.InputTokens == nil || u.OutputTokens == nil {
		return nil, errors.New("decisions response is missing usage")
	}
	for _, tokens := range []*int64{u.InputTokens, u.OutputTokens, u.TotalTokens} {
		if tokens != nil && (*tokens < 0 || *tokens > math.MaxInt32) {
			return nil, errors.New("invalid decisions token usage")
		}
	}
	total := *u.InputTokens + *u.OutputTokens
	if total > math.MaxInt32 || u.TotalTokens != nil && *u.TotalTokens != total {
		return nil, errors.New("inconsistent decisions total_tokens")
	}
	usage := &Usage{PromptTokens: int(*u.InputTokens), CompletionTokens: int(*u.OutputTokens), TotalTokens: int(total)}
	if u.InputDetails != nil {
		for _, tokens := range []*int64{u.InputDetails.CachedTokens, u.InputDetails.CacheWriteTokens} {
			if tokens != nil && (*tokens < 0 || *tokens > *u.InputTokens) {
				return nil, errors.New("invalid decisions cache usage")
			}
		}
		if u.InputDetails.CachedTokens != nil {
			usage.PromptTokensDetails.CachedTokens = int(*u.InputDetails.CachedTokens)
		}
		if u.InputDetails.CacheWriteTokens != nil {
			usage.PromptTokensDetails.CacheWriteTokens = int(*u.InputDetails.CacheWriteTokens)
		}
		if usage.PromptTokensDetails.CachedTokens+usage.PromptTokensDetails.CacheWriteTokens > usage.PromptTokens {
			return nil, errors.New("cache usage exceeds input_tokens")
		}
	}
	if u.OutputDetails != nil && u.OutputDetails.ReasoningTokens != nil {
		if *u.OutputDetails.ReasoningTokens < 0 || *u.OutputDetails.ReasoningTokens > *u.OutputTokens {
			return nil, errors.New("invalid decisions reasoning usage")
		}
		usage.CompletionTokenDetails.ReasoningTokens = int(*u.OutputDetails.ReasoningTokens)
	}
	switch request := request.(type) {
	case *OpenAIDecisionsRequest:
		if u.TotalTokens == nil {
			return nil, errors.New("OpenAI decisions response is missing total_tokens")
		}
		if u.InputDetails == nil || u.InputDetails.CachedTokens == nil || u.InputDetails.CacheWriteTokens == nil || u.OutputDetails == nil || u.OutputDetails.ReasoningTokens == nil {
			return nil, errors.New("OpenAI decisions response is missing token details")
		}
		var answers []map[string]json.RawMessage
		if err := kitutil.Unmarshal(response.Answers, &answers); err != nil || len(answers) != len(request.Questions) {
			return nil, errors.New("invalid OpenAI answers")
		}
		for i, answer := range answers {
			question := request.Questions[i]
			var name *string
			if _, exists := answer["name"]; !exists {
				return nil, errors.New("answer name is required")
			}
			if err := kitutil.Unmarshal(answer["name"], &name); err != nil {
				return nil, err
			}
			if (name == nil) != (question.Name == nil) || name != nil && *name != *question.Name {
				return nil, errors.New("answer name does not match question")
			}
			if err := validateDecisionAnswer(answer, question.Type, question.Choices, question.Levels, nil); err != nil {
				return nil, err
			}
		}
	case *JEVDecisionsRequest:
		var answers map[string]map[string]json.RawMessage
		if err := kitutil.Unmarshal(response.Answers, &answers); err != nil || len(answers) != len(request.Questions) {
			return nil, errors.New("invalid JEV answers")
		}
		for name, question := range request.Questions {
			answer, ok := answers[name]
			if !ok {
				return nil, errors.New("missing JEV answer")
			}
			if err := validateDecisionAnswer(answer, question.Type, nil, nil, question.Criteria); err != nil {
				return nil, err
			}
		}
	default:
		return nil, errors.New("unknown decisions protocol")
	}
	return usage, nil
}

func decisionNumber(raw json.RawMessage, upper float64) (float64, error) {
	var number *float64
	if err := kitutil.Unmarshal(raw, &number); err != nil || number == nil || math.IsNaN(*number) || math.IsInf(*number, 0) || *number < 0 || *number > upper {
		return 0, errors.New("invalid decisions answer value")
	}
	return *number, nil
}

func validateDecisionAnswer(answer map[string]json.RawMessage, expected string, choices []OpenAIDecisionChoice, levels []OpenAIDecisionLevel, criteria json.RawMessage) error {
	var kind string
	if err := kitutil.Unmarshal(answer["type"], &kind); err != nil {
		return err
	}
	if kind == "refusal" && criteria == nil && expected != "noul" {
		return nil
	}
	if kind != expected {
		return errors.New("answer type does not match question")
	}
	if kind == "predicate" || kind == "noul" {
		field := "probability"
		if kind == "noul" {
			field = "noul"
		}
		_, err := decisionNumber(answer[field], 1)
		return err
	}
	if _, err := decisionNumber(answer["confidence"], 1); err != nil {
		return err
	}
	if criteria != nil {
		var probabilities map[string]json.RawMessage
		if err := kitutil.Unmarshal(answer["probabilities"], &probabilities); err != nil {
			return err
		}
		var expectedKeys map[string]json.RawMessage
		if kind == "choice" {
			if err := kitutil.Unmarshal(criteria, &expectedKeys); err != nil {
				return err
			}
			var choice string
			if err := kitutil.Unmarshal(answer["choice"], &choice); err != nil {
				return err
			}
			if _, exists := expectedKeys[choice]; !exists {
				return errors.New("answer choice not in criteria")
			}
		} else {
			var levelValues []json.RawMessage
			if err := kitutil.Unmarshal(criteria, &levelValues); err != nil {
				return err
			}
			if _, err := decisionNumber(answer["score"], float64(len(levelValues)-1)); err != nil {
				return err
			}
			expectedKeys = make(map[string]json.RawMessage, len(levelValues))
			var legend map[string]string
			if err := kitutil.Unmarshal(answer["legend"], &legend); err != nil || len(legend) != len(levelValues) {
				return errors.New("invalid score legend")
			}
			for i := range levelValues {
				key := strconv.Itoa(i)
				expectedKeys[key] = nil
				if _, exists := legend[key]; !exists {
					return errors.New("missing score legend level")
				}
			}
		}
		if len(probabilities) != len(expectedKeys) {
			return errors.New("invalid probability distribution")
		}
		sum := 0.0
		for key := range expectedKeys {
			probability, err := decisionNumber(probabilities[key], 1)
			if err != nil {
				return err
			}
			sum += probability
		}
		if math.Abs(sum-1) > 0.001 {
			return errors.New("probabilities must sum to one")
		}
		return nil
	}
	var probabilities []struct {
		Value       any      `json:"value"`
		Label       *string  `json:"label"`
		Probability *float64 `json:"probability"`
	}
	if err := kitutil.Unmarshal(answer["probabilities"], &probabilities); err != nil {
		return err
	}
	count := len(choices)
	if kind == "score" {
		count = len(levels)
		if _, err := decisionNumber(answer["score"], float64(len(levels)-1)); err != nil {
			return err
		}
	} else {
		var choice any
		if err := kitutil.Unmarshal(answer["choice"], &choice); err != nil {
			return err
		}
		switch choice.(type) {
		case string, bool:
		default:
			return errors.New("answer choice must be a string or boolean")
		}
		found := false
		for _, option := range choices {
			if choice == option.Value {
				found = true
			}
		}
		if !found {
			return errors.New("answer choice not in choices")
		}
	}
	if len(probabilities) != count {
		return errors.New("invalid probability distribution")
	}
	seen := make(map[any]bool, count)
	sum := 0.0
	for _, probability := range probabilities {
		if probability.Probability == nil || *probability.Probability < 0 || *probability.Probability > 1 {
			return errors.New("invalid probability")
		}
		switch value := probability.Value.(type) {
		case bool, string:
			if kind != "choice" {
				return errors.New("score probability value must be an integer")
			}
			found := false
			for _, option := range choices {
				if value == option.Value {
					found = true
				}
			}
			if !found {
				return errors.New("unknown probability choice")
			}
		case float64:
			if kind != "score" || value != math.Trunc(value) || value < 0 || value >= float64(len(levels)) || probability.Label == nil || *probability.Label != levels[int(value)].Label {
				return errors.New("invalid score probability level")
			}
		default:
			return errors.New("invalid probability value")
		}
		if seen[probability.Value] {
			return errors.New("duplicate probability value")
		}
		seen[probability.Value] = true
		sum += *probability.Probability
	}
	if math.Abs(sum-1) > 0.001 {
		return errors.New("probabilities must sum to one")
	}
	return nil
}
