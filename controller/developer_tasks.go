package controller

import (
	"encoding/base64"
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

func GetDeveloperTasks(c *gin.Context) {
	token, authenticated := middleware.GetPublicAPIKey(c)
	if !authenticated {
		videoProxyError(c, http.StatusNotFound, "invalid_request_error", "Task not found")
		return
	}
	query := c.Request.URL.Query()
	limit := 20
	if values, supplied := query["limit"]; supplied {
		if len(values) != 1 || values[0] == "" {
			videoProxyError(c, http.StatusBadRequest, "invalid_request_error", "Invalid task limit")
			return
		}
		parsed, err := strconv.Atoi(values[0])
		if err != nil || parsed < 1 || parsed > 100 {
			videoProxyError(c, http.StatusBadRequest, "invalid_request_error", "limit must be between 1 and 100")
			return
		}
		limit = parsed
	}
	var beforeID int64
	if values, supplied := query["cursor"]; supplied {
		if len(values) != 1 || values[0] == "" {
			videoProxyError(c, http.StatusBadRequest, "invalid_request_error", "Invalid task cursor")
			return
		}
		cursor := values[0]
		if len(cursor) > 40 {
			videoProxyError(c, http.StatusBadRequest, "invalid_request_error", "Invalid task cursor")
			return
		}
		decoded, err := base64.RawURLEncoding.DecodeString(cursor)
		value, valid := strings.CutPrefix(string(decoded), "task:")
		if err == nil && valid {
			beforeID, err = strconv.ParseInt(value, 10, 64)
		}
		if err != nil || !valid || beforeID <= 0 {
			videoProxyError(c, http.StatusBadRequest, "invalid_request_error", "Invalid task cursor")
			return
		}
	}
	tasks, nextID, err := model.ListTasksForToken(c.Request.Context(), token.UserId, token.Id, beforeID, limit)
	if err != nil {
		videoProxyError(c, http.StatusInternalServerError, "server_error", "Failed to query tasks")
		return
	}
	response := dto.DeveloperTaskList{Object: "list", Data: make([]dto.DeveloperTask, 0, len(tasks)), HasMore: nextID > 0}
	for _, task := range tasks {
		response.Data = append(response.Data, developerTaskView(task))
	}
	if nextID > 0 {
		response.NextCursor = base64.RawURLEncoding.EncodeToString([]byte("task:" + strconv.FormatInt(nextID, 10)))
	}
	c.JSON(http.StatusOK, response)
}

func GetDeveloperTask(c *gin.Context) {
	token, authenticated := middleware.GetPublicAPIKey(c)
	if !authenticated {
		videoProxyError(c, http.StatusNotFound, "invalid_request_error", "Task not found")
		return
	}
	task, exists, err := model.GetTaskForTokenWithContext(c.Request.Context(), token.UserId, token.Id, c.Param("key"))
	if err != nil {
		videoProxyError(c, http.StatusInternalServerError, "server_error", "Failed to query task")
		return
	}
	if !exists {
		videoProxyError(c, http.StatusNotFound, "invalid_request_error", "Task not found")
		return
	}
	c.JSON(http.StatusOK, developerTaskView(task))
}

func developerTaskView(task *model.Task) dto.DeveloperTask {
	createdAt := task.CreatedAt
	if createdAt == 0 {
		createdAt = task.SubmitTime
	}
	view := dto.DeveloperTask{Object: "task", ID: task.TaskID, Type: string(task.Platform), Model: task.Properties.OriginModelName, Status: task.Status.ToVideoStatus(), Progress: task.Progress, CreatedAt: createdAt, RequestID: task.RequestId}
	progress, err := strconv.Atoi(strings.TrimSuffix(task.Progress, "%"))
	if err != nil || progress < 0 || progress > 100 {
		view.Progress = ""
	} else {
		view.Progress = strconv.Itoa(progress) + "%"
	}
	if task.StartTime > 0 {
		value := task.StartTime
		view.StartedAt = &value
	}
	if task.FinishTime > 0 {
		value := task.FinishTime
		view.CompletedAt = &value
	}
	if task.Status == model.TaskStatusFailure {
		view.Error = &dto.DeveloperTaskError{Code: "task_failed", Message: "The task failed."}
	}
	return view
}
