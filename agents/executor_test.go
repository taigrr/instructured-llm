package agents_test

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tmc/langchaingo/agents"
	"github.com/tmc/langchaingo/chains"
	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/openai"
	"github.com/tmc/langchaingo/prompts"
	"github.com/tmc/langchaingo/schema"
	"github.com/tmc/langchaingo/tools"
	"github.com/tmc/langchaingo/tools/serpapi"
)

type testAgent struct {
	actions    []schema.AgentAction
	finish     *schema.AgentFinish
	err        error
	inputKeys  []string
	outputKeys []string

	recordedIntermediateSteps []schema.AgentStep
	recordedInputs            map[string]any
	numPlanCalls              int
}

func (a *testAgent) Plan(
	_ context.Context,
	intermediateSteps []schema.AgentStep,
	inputs map[string]any, _ []llms.ChatMessage,
) ([]schema.AgentAction, *schema.AgentFinish, []llms.ChatMessage, error) {
	a.recordedIntermediateSteps = intermediateSteps
	a.recordedInputs = inputs
	a.numPlanCalls++

	return a.actions, a.finish, nil, a.err
}

func (a *testAgent) GetInputKeys() []string {
	return a.inputKeys
}

func (a *testAgent) GetOutputKeys() []string {
	return a.outputKeys
}

func (a *testAgent) GetTools() []tools.Tool {
	return nil
}

type recordingTool struct {
	recordedInput string
}

func (t *recordingTool) Name() string        { return "recorder" }
func (t *recordingTool) Description() string { return "records the input it receives" }

func (t *recordingTool) Call(_ context.Context, input string) (string, error) {
	t.recordedInput = input
	return "ok", nil
}

// toolCallingAgent is a minimal Agent used to exercise Executor.doAction
// through the public Executor.Call API.
type toolCallingAgent struct {
	tools     []tools.Tool
	toolInput string
}

func (a *toolCallingAgent) Plan(
	_ context.Context,
	_ []schema.AgentStep,
	_ map[string]any, _ []llms.ChatMessage,
) ([]schema.AgentAction, *schema.AgentFinish, []llms.ChatMessage, error) {
	return []schema.AgentAction{
		{Tool: "recorder", ToolInput: a.toolInput},
	}, nil, nil, nil
}

func (a *toolCallingAgent) GetInputKeys() []string  { return nil }
func (a *toolCallingAgent) GetOutputKeys() []string { return []string{"output"} }
func (a *toolCallingAgent) GetTools() []tools.Tool  { return a.tools }

func TestExecutorTrimsHallucinatedObservationFromToolInput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		toolIn   string
		expected string
	}{
		{
			name:     "no trailing Observation",
			toolIn:   "do the thing",
			expected: "do the thing",
		},
		{
			name:     "trailing newline Observation",
			toolIn:   "do the thing\nObservation:",
			expected: "do the thing",
		},
		{
			name:     "trailing tab-indented Observation",
			toolIn:   "do the thing\n\tObservation:",
			expected: "do the thing",
		},
		{
			name:     "trailing Observation with extra whitespace",
			toolIn:   "do the thing\nObservation: \n",
			expected: "do the thing",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tool := &recordingTool{}
			a := &toolCallingAgent{tools: []tools.Tool{tool}, toolInput: tt.toolIn}
			executor := agents.NewExecutor(a, agents.WithMaxIterations(1))

			_, err := chains.Call(context.Background(), executor, nil)
			require.ErrorIs(t, err, agents.ErrNotFinished)
			require.Equal(t, tt.expected, tool.recordedInput)
		})
	}
}

func TestExecutorWithErrorHandler(t *testing.T) {
	t.Parallel()

	a := &testAgent{
		err: agents.ErrUnableToParseOutput,
	}
	executor := agents.NewExecutor(
		a,
		agents.WithMaxIterations(3),
		agents.WithParserErrorHandler(agents.NewParserErrorHandler(nil)),
	)

	_, err := chains.Call(context.Background(), executor, nil)
	require.ErrorIs(t, err, agents.ErrNotFinished)
	require.Equal(t, 3, a.numPlanCalls)
	require.Equal(t, []schema.AgentStep{
		{Observation: agents.ErrUnableToParseOutput.Error()},
		{Observation: agents.ErrUnableToParseOutput.Error()},
	}, a.recordedIntermediateSteps)
}

func TestExecutorWithMRKLAgent(t *testing.T) {
	t.Parallel()

	if openaiKey := os.Getenv("OPENAI_API_KEY"); openaiKey == "" {
		t.Skip("OPENAI_API_KEY not set")
	}
	if serpapiKey := os.Getenv("SERPAPI_API_KEY"); serpapiKey == "" {
		t.Skip("SERPAPI_API_KEY not set")
	}

	llm, err := openai.New()
	require.NoError(t, err)

	searchTool, err := serpapi.New()
	require.NoError(t, err)

	calculator := tools.Calculator{}

	a, err := agents.Initialize(
		llm,
		[]tools.Tool{searchTool, calculator},
		agents.ZeroShotReactDescription,
	)
	require.NoError(t, err)

	result, err := chains.Run(context.Background(), a, "If a person lived three times as long as Jacklyn Zeman, how long would they live") //nolint:lll
	require.NoError(t, err)

	require.True(t, strings.Contains(result, "210"), "correct answer 210 not in response")
}

func TestExecutorWithOpenAIFunctionAgent(t *testing.T) {
	t.Parallel()

	if openaiKey := os.Getenv("OPENAI_API_KEY"); openaiKey == "" {
		t.Skip("OPENAI_API_KEY not set")
	}
	if serpapiKey := os.Getenv("SERPAPI_API_KEY"); serpapiKey == "" {
		t.Skip("SERPAPI_API_KEY not set")
	}

	llm, err := openai.New()
	require.NoError(t, err)

	searchTool, err := serpapi.New()
	require.NoError(t, err)

	calculator := tools.Calculator{}

	toolList := []tools.Tool{searchTool, calculator}

	a := agents.NewOpenAIFunctionsAgent(llm,
		toolList,
		agents.NewOpenAIOption().WithSystemMessage("you are a helpful assistant"),
		agents.NewOpenAIOption().WithExtraMessages([]prompts.MessageFormatter{
			prompts.NewHumanMessagePromptTemplate("please be strict", nil),
		}),
	)

	e := agents.NewExecutor(a)
	require.NoError(t, err)

	result, err := chains.Run(context.Background(), e, "what is HK singer Eason Chan's years old?") //nolint:lll
	require.NoError(t, err)

	require.True(t, strings.Contains(result, "47") || strings.Contains(result, "49"),
		"correct answer 47 or 49 not in response")
}
