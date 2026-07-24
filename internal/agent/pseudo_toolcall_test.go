package agent

import "testing"

func TestLooksLikePseudoToolCall(t *testing.T) {
	cases := []struct {
		name    string
		content string
		want    bool
	}{
		{name: "empty", content: "", want: false},
		{name: "normal summary", content: "Implemented the rename archive as discussed.", want: false},
		{name: "mentions read_file in prose", content: "I used read_file to inspect README.md.", want: false},
		{
			name: "deepseek DSML",
			content: `<|DSML|tool_calls>
<|DSML|invoke name="read_file">
<|DSML|parameter name="path" string="true">docs/RENAME-TO-MATEA.md</|DSML|parameter>
</|DSML|invoke>
</|DSML|tool_calls>`,
			want: true,
		},
		{
			name:    "xml tool_call",
			content: `<tool_call>{"name":"read_file","arguments":{"path":"a.md"}}</tool_call>`,
			want:    true,
		},
		{
			name:    "invoke name attribute",
			content: `invoke name="write_file"`,
			want:    true,
		},
		{
			name:    "function tags",
			content: `<function name="read_file">{"path":"x"}</function>`,
			want:    true,
		},
		// Documented non-coverage: bare JSON in content must stay false so
		// maintainers do not assume the detector handles every vendor format.
		{
			name:    "bare openai-shaped json in content (unknown / not covered)",
			content: `{"name":"read_file","arguments":{"path":"docs/RENAME-TO-MATEA.md"}}`,
			want:    false,
		},
		{
			name:    "chatglm-ish tools_call without known markers (unknown / not covered)",
			content: `tools_call: read_file(path=README.md)`,
			want:    false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := LooksLikePseudoToolCall(tc.content)
			if got != tc.want {
				t.Fatalf("LooksLikePseudoToolCall()=%v, want %v", got, tc.want)
			}
		})
	}
}
