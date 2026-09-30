package service

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	pluginruntime "github.com/QuantumNous/new-api/pkg/jsplugin"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
)

func CaptureTaskSubmission(c *gin.Context, info *relaycommon.RelayInfo, task *model.Task) {
	if task == nil || info == nil || info.ChannelMeta == nil {
		return
	}
	execution := TaskExecutionSnapshotFromContext(c)
	if execution == nil {
		execution = &model.TaskExecutionSnapshot{}
	}
	capture := &model.RuntimeEvidence{ClientModel: info.OriginModelName, ChannelID: info.ChannelId, ChannelType: info.ChannelType, UpstreamModel: info.UpstreamModelName, LocalRequestID: execution.RequestID, RequestID: info.PassiveRequestID, TraceID: info.PassiveTraceID, TaskID: task.PrivateData.UpstreamTaskID, SubmittedAt: task.SubmitTime, Endpoint: execution.RequestPath, UpstreamEndpoint: info.PassiveEndpoint, Protocol: string(info.GetFinalRequestRelayFormat())}
	if execution.TaskPlugin != nil {
		capture.PluginKey = execution.TaskPlugin.Key
	}
	if value, ok := c.Get(pluginruntime.ContextKeyPinnedEndpoint); ok {
		if pinned, ok := value.(pluginruntime.PinnedEndpoint); ok {
			capture.Protocol = string(pinned.Protocol)
		}
	}
	capture.Requested = RuntimeRequestFacts(c, info)
	execution.Passive = capture
	task.PrivateData.Execution = execution
	if task.Status == model.TaskStatusSuccess || task.Status == model.TaskStatusFailure {
		model.CaptureTaskRuntime(task, nil, task.Data)
	}
}

func AppendPassiveTextEvidence(c *gin.Context, info *relaycommon.RelayInfo, usage *dto.Usage, other *model.LogOther) {
	if info == nil || info.ChannelMeta == nil || other == nil {
		return
	}
	capture := model.RuntimeEvidence{ClientModel: info.OriginModelName, ChannelID: info.ChannelId, ChannelType: info.ChannelType, UpstreamModel: info.UpstreamModelName, LocalRequestID: info.RequestId, RequestID: info.PassiveRequestID, TraceID: info.PassiveTraceID, UpstreamEndpoint: info.PassiveEndpoint, Protocol: string(info.GetFinalRequestRelayFormat()), SubmittedAt: info.StartTime.Unix()}
	if c.Request != nil && c.Request.URL != nil {
		capture.Endpoint = c.Request.URL.Path
	}
	capture.Requested = RuntimeRequestFacts(c, info)
	body, err := common.Marshal(usage)
	if err == nil {
		capture.Terminal = model.RuntimeBillingFacts(body)
	}
	if usage != nil && usage.NumServerSideToolsUsed != nil {
		capture.Terminal["server_tool_calls"] = *usage.NumServerSideToolsUsed
	}
	if info.ResponsesUsageInfo != nil {
		tools := map[string]any{}
		for name, tool := range info.ResponsesUsageInfo.BuiltInTools {
			if tool != nil && (name == "image_generation" || name == "web_search" || name == "web_search_preview") {
				tools[name] = tool.CallCount
			}
		}
		capture.Terminal["tool_outcomes"] = tools
	}
	other.SetRoot("passive_runtime_binding", capture)
}

func RuntimeRequestFacts(c *gin.Context, info *relaycommon.RelayInfo) map[string]any {
	if info.PassiveRequestFacts != nil {
		return info.PassiveRequestFacts
	}
	if info.BillingRequestInput != nil {
		return model.RuntimeBillingFacts(info.BillingRequestInput.Body)
	}
	if c == nil || c.Request == nil {
		return nil
	}
	storage, err := common.GetBodyStorage(c)
	if err != nil || storage.Size() > 1<<20 {
		return nil
	}
	body, err := storage.Bytes()
	if err != nil {
		return nil
	}
	return model.RuntimeBillingFacts(body)
}
