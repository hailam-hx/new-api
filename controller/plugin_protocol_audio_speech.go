package controller

import (
	"context"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	pluginruntime "github.com/QuantumNous/new-api/pkg/jsplugin"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

// Speech submissions return a public task ID after the ordinary durable task
// barrier. The bounded submission survives a disconnected client; subsequent
// settlement belongs to the regular background task poller.
func serveTaskPluginAudioSpeech(c *gin.Context, pinned pluginruntime.PinnedEndpoint, deps pluginProtocolBridgeDeps) {
	deps = deps.withDefaults()
	if pinned.Protocol != pluginruntime.ProtocolOpenAIAudioSpeech || pinned.Plugin == nil {
		respondPluginProtocolError(c, http.StatusInternalServerError, "task_protocol_error", "Task protocol request failed")
		return
	}
	clientRequest := c.Request
	submissionContext, cancel := context.WithTimeout(context.WithoutCancel(clientRequest.Context()), deps.submissionTimeout)
	defer cancel()
	c.Request = clientRequest.Clone(submissionContext)
	defer func() { c.Request = clientRequest }()
	relayInfo, err := relaycommon.GenRelayInfo(c, types.RelayFormatTask, nil, nil)
	if err != nil {
		logger.LogError(c, "build audio speech task relay info failed: "+err.Error())
		respondPluginProtocolError(c, http.StatusInternalServerError, "task_protocol_error", "Task protocol request failed")
		return
	}
	relayInfo.OriginModelName = c.GetString("resolved_task_model")
	relayInfo.Action = c.GetString("task_action")
	channelID := common.GetContextKeyInt(c, constant.ContextKeyChannelId)
	channel, channelErr := model.GetChannelById(channelID, false)
	if channelErr != nil || channel == nil || !channel.GetSetting().BindsTaskPlugin("dflop-tts") {
		respondPluginProtocolError(c, http.StatusBadRequest, "task_channel_invalid", "Selected channel is not bound to DFLOP speech")
		return
	}
	if relayInfo.TaskRelayInfo == nil {
		relayInfo.TaskRelayInfo = &relaycommon.TaskRelayInfo{}
	}
	relayInfo.LockedChannel = channel
	outcome, taskErr := deps.submit(c, relayInfo)
	if clientRequest.Context().Err() != nil {
		return
	}
	if taskErr != nil {
		respondTaskPluginImageError(c, taskErr)
		return
	}
	if outcome == nil || outcome.Task == nil || outcome.Task.Platform != constant.TaskPlatform(pinned.Plugin.Meta.Key) {
		respondPluginProtocolError(c, http.StatusInternalServerError, "task_protocol_error", "Task protocol request failed")
		return
	}
	presentTaskSubmission(c, outcome)
}

// RetrieveTaskPluginAudioSpeech never queries a provider with a client supplied
// ID. The task is looked up under the authenticated user and returned from the
// host's persisted view, which excludes the private upstream ID and key.
func RetrieveTaskPluginAudioSpeech(c *gin.Context) {
	taskID := strings.TrimSpace(c.Param("task_id"))
	userID := common.GetContextKeyInt(c, constant.ContextKeyUserId)
	if !strings.HasPrefix(taskID, "task_") || userID <= 0 {
		respondPluginProtocolError(c, http.StatusNotFound, "task_not_found", "Task not found")
		return
	}
	task, found, err := model.GetByTaskId(userID, taskID)
	if err != nil {
		respondPluginProtocolError(c, http.StatusInternalServerError, "task_protocol_error", "Task lookup failed")
		return
	}
	if !found || task == nil || task.Platform != "dflop-tts" || task.Properties.OriginModelName != "voice-tts-pro" || !task.ResultRetrievable() {
		respondPluginProtocolError(c, http.StatusNotFound, "task_not_found", "Task not found")
		return
	}
	view, err := service.BuildTaskPluginView(task)
	if err != nil {
		respondPluginProtocolError(c, http.StatusInternalServerError, "task_protocol_error", "Task view failed")
		return
	}
	c.JSON(http.StatusOK, view)
}
