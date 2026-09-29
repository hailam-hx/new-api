package controller

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPostSetupUsernameValidation(t *testing.T) {
	dialect := os.Getenv("TEST_SETUP_DIALECT")
	if dialect == "" {
		dialect = "sqlite"
	}
	require.Contains(t, []string{"sqlite", "mysql", "postgres"}, dialect)
	dsn := os.Getenv("TEST_" + strings.ToUpper(dialect) + "_DSN")

	for _, tc := range []struct {
		name      string
		username  string
		wantOK    bool
		wantLimit bool
	}{
		{name: "requested 13 character username", username: "hotx_official", wantOK: true},
		{name: "20 character username", username: "abcdefghijklmnopqrst", wantOK: true},
		{name: "21 character username", username: "abcdefghijklmnopqrstu", wantLimit: true},
		{name: "empty username", username: ""},
		{name: "whitespace username", username: "   "},
	} {
		t.Run(tc.name, func(t *testing.T) {
			previousDB := model.DB
			previousSetup := constant.Setup
			previousOptions := common.OptionMap
			previousSelfUse := operation_setting.SelfUseModeEnabled
			previousDemo := operation_setting.DemoSiteEnabled
			db, _ := newAuditTestDatabase(t, dialect, dsn)
			require.NoError(t, db.AutoMigrate(&model.User{}, &model.Option{}, &model.Setup{}))
			model.DB = db
			constant.Setup = false
			common.OptionMap = make(map[string]string)
			t.Cleanup(func() {
				model.DB = previousDB
				constant.Setup = previousSetup
				common.OptionMap = previousOptions
				operation_setting.SelfUseModeEnabled = previousSelfUse
				operation_setting.DemoSiteEnabled = previousDemo
				sqlDB, err := db.DB()
				if err == nil {
					_ = sqlDB.Close()
				}
			})

			request := SetupRequest{
				Username: tc.username, Password: "valid-test-password", ConfirmPassword: "valid-test-password",
			}
			response := postSetupTestRequest(t, request)
			assert.Equal(t, tc.wantOK, response.Success)
			if tc.wantLimit {
				assert.Contains(t, response.Message, "20")
			}

			var users []model.User
			require.NoError(t, db.Find(&users).Error)
			if !tc.wantOK {
				assert.Empty(t, users)
				assert.False(t, constant.Setup)
				return
			}

			require.Len(t, users, 1)
			assert.Equal(t, tc.username, users[0].Username)
			assert.NotEqual(t, request.Password, users[0].Password)
			assert.Equal(t, common.RoleRootUser, users[0].Role)
			assert.True(t, constant.Setup)

			replay := postSetupTestRequest(t, request)
			assert.False(t, replay.Success)
			var count int64
			require.NoError(t, db.Model(&model.User{}).Count(&count).Error)
			assert.EqualValues(t, 1, count)
		})
	}
}

func postSetupTestRequest(t *testing.T, payload SetupRequest) struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
} {
	t.Helper()
	body, err := common.Marshal(payload)
	require.NoError(t, err)
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodPost, "/api/setup", strings.NewReader(string(body)))
	context.Request.Header.Set("Content-Type", "application/json")
	PostSetup(context)
	require.Equal(t, http.StatusOK, recorder.Code)
	var response struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	return response
}
