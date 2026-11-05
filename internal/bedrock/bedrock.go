// Package bedrock wraps AWS Bedrock so the rdq TUI can ask an LLM to
// translate natural language into SQL.
//
// It uses the unified Converse API rather than InvokeModel so the same code
// path works for any text-capable model the user picks. Inference profiles
// are preferred over direct foundation models because they fail over across
// regions automatically.
package bedrock

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/bedrock"
	bedrocktypes "github.com/aws/aws-sdk-go-v2/service/bedrock/types"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	rttypes "github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
)

// ErrTruncated reports that the model stopped because it hit the
// max-token limit, so the reply is incomplete.
var ErrTruncated = errors.New("bedrock response was truncated at the max token limit; try a narrower request")

// Client groups the two Bedrock SDK clients we need: the management client
// for listing models and the runtime client for actually invoking them.
type Client struct {
	rt *bedrockruntime.Client
	bd *bedrock.Client
}

// ModelInfo is a flattened, user-friendly view of an inference profile or
// foundation model entry.
type ModelInfo struct {
	ID          string // ARN or model identifier accepted by Converse
	Name        string // human-friendly label
	Description string
}

// Role is the speaker of a single Message in a multi-turn conversation.
// Only User and Assistant are supported — system instructions are passed
// separately via the systemPrompt argument.
type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
)

// Message is a single turn in a multi-turn conversation. Both Ask and
// Explain accept a slice of these to keep context across calls within a
// single TUI session.
type Message struct {
	Role Role
	Text string
}

// New constructs a Client from a resolved AWS config. The Bedrock service
// must be enabled in the configured region; the call is lazy so this
// function never fails.
func New(cfg aws.Config) *Client {
	return &Client{
		rt: bedrockruntime.NewFromConfig(cfg),
		bd: bedrock.NewFromConfig(cfg),
	}
}

// ListModels returns the inference profiles available in the configured
// region. Cross-region inference profiles (the "us." / "apac." prefixed
// entries) are preferred because they automatically fail over across
// regions, which is essential for high-traffic Claude usage. Falls back to
// foundation models if no profiles are returned.
func (c *Client) ListModels(ctx context.Context) ([]ModelInfo, error) {
	profiles, err := c.listInferenceProfiles(ctx)
	if err == nil && len(profiles) > 0 {
		return profiles, nil
	}
	if err != nil {
		// Surface the inference profile error only if the fallback also
		// fails. Some regions return AccessDeniedException for profiles
		// even when foundation models work.
		fallback, fbErr := c.listFoundationModels(ctx)
		if fbErr == nil && len(fallback) > 0 {
			return fallback, nil
		}
		return nil, fmt.Errorf("list bedrock models: %w", err)
	}
	return c.listFoundationModels(ctx)
}

func (c *Client) listInferenceProfiles(ctx context.Context) ([]ModelInfo, error) {
	var out []ModelInfo
	paginator := bedrock.NewListInferenceProfilesPaginator(c.bd, &bedrock.ListInferenceProfilesInput{})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		for _, p := range page.InferenceProfileSummaries {
			out = append(out, ModelInfo{
				ID:          aws.ToString(p.InferenceProfileId),
				Name:        aws.ToString(p.InferenceProfileName),
				Description: aws.ToString(p.Description),
			})
		}
	}
	return out, nil
}

func (c *Client) listFoundationModels(ctx context.Context) ([]ModelInfo, error) {
	resp, err := c.bd.ListFoundationModels(ctx, &bedrock.ListFoundationModelsInput{
		ByOutputModality: bedrocktypes.ModelModalityText,
	})
	if err != nil {
		return nil, fmt.Errorf("list foundation models: %w", err)
	}
	var out []ModelInfo
	for _, m := range resp.ModelSummaries {
		// Skip embedding-only and image models.
		if !supportsTextOutput(m.OutputModalities) {
			continue
		}
		out = append(out, ModelInfo{
			ID:          aws.ToString(m.ModelId),
			Name:        aws.ToString(m.ModelName),
			Description: aws.ToString(m.ProviderName),
		})
	}
	return out, nil
}

func supportsTextOutput(modalities []bedrocktypes.ModelModality) bool {
	for _, m := range modalities {
		if m == bedrocktypes.ModelModalityText {
			return true
		}
	}
	return false
}

// Ask sends a multi-turn conversation to the chosen model and returns the
// assistant's text response with markdown code fences stripped. The caller
// owns the conversation slice — pass an empty / nil slice for a brand-new
// session and append both the user prompt and the model's reply between
// calls to keep context across turns.
func (c *Client) Ask(ctx context.Context, modelID, systemPrompt string, messages []Message) (string, error) {
	out, err := c.converse(ctx, modelID, systemPrompt, messages)
	if err != nil {
		return "", err
	}
	return stripCodeFence(out), nil
}

// Explain sends a multi-turn conversation and returns the raw text
// untouched, so the response can include markdown bullet lists and ```sql
// fenced examples for the error analysis viewport.
//
// Unlike Ask, a reply truncated at the token limit is still returned (with
// a trailing note): a partial prose explanation is useful, whereas partial
// SQL is dangerous.
func (c *Client) Explain(ctx context.Context, modelID, systemPrompt string, messages []Message) (string, error) {
	out, err := c.converse(ctx, modelID, systemPrompt, messages)
	if errors.Is(err, ErrTruncated) && strings.TrimSpace(out) != "" {
		return out + truncatedNote, nil
	}
	return out, err
}

// truncatedNote is appended to prose replies that hit the token limit.
const truncatedNote = "\n\n_(response truncated at the token limit)_"

// converse is the shared transport behind Ask and Explain. It serializes
// the supplied conversation into Bedrock's Converse Messages format and
// concatenates every text content block from the assistant's reply.
func (c *Client) converse(ctx context.Context, modelID, systemPrompt string, messages []Message) (string, error) {
	if modelID == "" {
		return "", errors.New("no bedrock model selected")
	}
	if len(messages) == 0 {
		return "", errors.New("empty conversation")
	}
	last := messages[len(messages)-1]
	if last.Role != RoleUser || strings.TrimSpace(last.Text) == "" {
		return "", errors.New("conversation must end with a non-empty user message")
	}

	bedrockMsgs := make([]rttypes.Message, 0, len(messages))
	for _, m := range messages {
		if strings.TrimSpace(m.Text) == "" {
			continue
		}
		role := rttypes.ConversationRoleUser
		if m.Role == RoleAssistant {
			role = rttypes.ConversationRoleAssistant
		}
		bedrockMsgs = append(bedrockMsgs, rttypes.Message{
			Role: role,
			Content: []rttypes.ContentBlock{
				&rttypes.ContentBlockMemberText{Value: m.Text},
			},
		})
	}

	out, err := c.rt.Converse(ctx, &bedrockruntime.ConverseInput{
		ModelId: aws.String(modelID),
		System: []rttypes.SystemContentBlock{
			&rttypes.SystemContentBlockMemberText{Value: systemPrompt},
		},
		Messages:        bedrockMsgs,
		InferenceConfig: inferenceConfigurationForModel(modelID),
	})
	if err != nil {
		return "", fmt.Errorf("bedrock converse: %w", err)
	}

	return converseReplyText(out)
}

// converseReplyText extracts the assistant's text from a Converse response,
// rejecting replies that are empty, truncated, or blocked.
func converseReplyText(out *bedrockruntime.ConverseOutput) (string, error) {
	if out == nil {
		return "", errors.New("bedrock returned no output")
	}
	msg, ok := out.Output.(*rttypes.ConverseOutputMemberMessage)
	if !ok {
		return "", errors.New("bedrock returned an unexpected output variant")
	}
	if len(msg.Value.Content) == 0 {
		return "", errors.New("bedrock returned an empty response")
	}

	// A reply blocked by a safety filter is not a usable answer. A reply
	// cut off by the token cap is reported via ErrTruncated so a truncated
	// SQL statement never reaches the editor (or the auto-run path) as if
	// it were complete.
	switch out.StopReason {
	case rttypes.StopReasonContentFiltered, rttypes.StopReasonGuardrailIntervened:
		return "", fmt.Errorf("bedrock response was blocked (stop reason: %s)", out.StopReason)
	}

	var sb strings.Builder
	for _, block := range msg.Value.Content {
		if t, ok := block.(*rttypes.ContentBlockMemberText); ok {
			sb.WriteString(t.Value)
		}
	}
	if out.StopReason == rttypes.StopReasonMaxTokens {
		// Return the partial text alongside the sentinel so prose callers
		// (Explain) can still show it; Ask surfaces the error instead.
		return sb.String(), ErrTruncated
	}
	if strings.TrimSpace(sb.String()) == "" {
		return "", errors.New("bedrock returned a response with no text content")
	}
	return sb.String(), nil
}

func inferenceConfigurationForModel(modelID string) *rttypes.InferenceConfiguration {
	cfg := &rttypes.InferenceConfiguration{
		MaxTokens: aws.Int32(2048),
	}
	if supportsTemperature(modelID) {
		cfg.Temperature = aws.Float32(0.2)
	}
	return cfg
}

func supportsTemperature(modelID string) bool {
	id := strings.ToLower(modelID)
	return !strings.Contains(id, "claude-opus-4-7") &&
		!strings.Contains(id, "claude-opus-4.7") &&
		!strings.Contains(id, "claude-opus-4-8") &&
		!strings.Contains(id, "claude-opus-4.8")
}

// stripCodeFence removes a leading and trailing markdown code fence so that
// the raw SQL can be pasted directly into the editor. Handles the common
// "```sql" and "```" variants.
func stripCodeFence(s string) string {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "```") {
		return s
	}
	// drop the opening fence + optional language tag
	if idx := strings.Index(s, "\n"); idx > 0 {
		s = s[idx+1:]
	} else {
		s = strings.TrimPrefix(s, "```")
	}
	// drop trailing fence
	s = strings.TrimRight(s, " \n")
	if strings.HasSuffix(s, "```") {
		s = strings.TrimSuffix(s, "```")
	}
	return strings.TrimSpace(s)
}
