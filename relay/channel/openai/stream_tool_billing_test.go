package openai

import (
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestObserveStreamChoicesDedupesFunctionCallNames(t *testing.T) {
	info := &relaycommon.RelayInfo{StreamStatus: relaycommon.NewStreamStatus()}
	seen := make(map[string]struct{})
	var names []string

	chunks := []string{
		`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"c1","type":"function","function":{"name":"get_weather","arguments":""}}]}}]}`,
		`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"q\":"}}]}}]}`,
		`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"x\"}"}}]}}]}`,
		`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":1,"id":"c2","type":"function","function":{"name":"get_time","arguments":""}}]}}]}`,
		`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":1,"function":{"arguments":"{}"}}]}}]}`,
	}
	for _, chunk := range chunks {
		observeStreamChoices(info, chunk, seen, &names)
	}

	require.Len(t, names, 2)
	assert.Equal(t, []string{"get_weather", "get_time"}, names)
	assert.Empty(t, info.StreamStatus.ResponseOutcome(), "tool call deltas carry no finish reason")

	observeStreamChoices(info, `{"choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`, seen, &names)
	assert.Equal(t, "completed", info.StreamStatus.ResponseOutcome())
	assert.False(t, info.PerformanceBusinessRejection)

	filtered := &relaycommon.RelayInfo{StreamStatus: relaycommon.NewStreamStatus()}
	observeStreamChoices(filtered, `{"choices":[{"index":0,"delta":{},"finish_reason":"content_filter"}]}`, map[string]struct{}{}, &names)
	assert.True(t, filtered.PerformanceBusinessRejection)
}

func TestChatFinalUsageCarriesServerToolCount(t *testing.T) {
	for _, tc := range []struct {
		name     string
		frame    string
		want     int
		hasCount bool
	}{
		{name: "reported count", frame: `{"usage":{"prompt_tokens":10,"completion_tokens":2,"num_server_side_tools_used":4}}`, want: 4, hasCount: true},
		{name: "explicit zero", frame: `{"usage":{"prompt_tokens":10,"completion_tokens":2,"num_server_side_tools_used":0}}`, hasCount: true},
		{name: "missing count", frame: `{"usage":{"prompt_tokens":10,"completion_tokens":2}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var id, model, fingerprint string
			var created int64
			var usage *dto.Usage
			var contains, send bool
			info := &relaycommon.RelayInfo{}
			require.NoError(t, handleLastResponse(tc.frame, &id, &created, &fingerprint, &model, &usage, &contains, info, &send))
			require.True(t, contains)
			require.NotNil(t, usage)
			if !tc.hasCount {
				assert.Nil(t, usage.NumServerSideToolsUsed)
			} else {
				require.NotNil(t, usage.NumServerSideToolsUsed)
				assert.Equal(t, tc.want, *usage.NumServerSideToolsUsed)
			}
		})
	}
}
