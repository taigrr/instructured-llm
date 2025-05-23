package structfmt

import (
	_ "embed"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/tmc/langchaingo/llms/openai"
)

//go:embed test_data/simple_struct_res.json
var simpleStructResp []byte

func TestStructToOAIRespFormat(t *testing.T) {
	tests := []struct {
		name string
		in   any
		out  []byte
	}{
		{
			name: "simple_struct_res",
			in: struct {
				Name string `json:"name" description:"Name of the person"`
				Age  int    `json:"age" description:"Age of the person"`
			}{
				Name: "John",
				Age:  30,
			},
			out: simpleStructResp,
		},
		// Other internal test cases have been removed

	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out := StructToOAIRespFormat(tc.in)
			assert.NotNil(t, out)

			jBytes, err := json.Marshal(out)
			assert.NoError(t, err)

			var expected openai.ResponseFormat
			err = json.Unmarshal(tc.out, &expected)
			assert.NoError(t, err)

			var result openai.ResponseFormat
			err = json.Unmarshal(jBytes, &result)
			assert.NoError(t, err)

			assert.Equal(t, expected, result)
		})
	}
}
