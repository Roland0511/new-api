package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAdminUserListsIncludeReadOnlySubscriptionStatus(t *testing.T) {
	db := setupManageUserTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.SubscriptionPlan{}, &model.UserSubscription{}))
	users := []model.User{
		{Id: 101, Username: "subscribed", AffCode: "sub1", Password: "secret", Group: "default"},
		{Id: 102, Username: "unsubscribed", AffCode: "sub2", Password: "secret", Group: "default"},
		{Id: 103, Username: "other-page", AffCode: "sub3", Password: "secret", Group: "default"},
	}
	require.NoError(t, db.Create(&users).Error)
	require.NoError(t, db.Create(&model.SubscriptionPlan{Id: 1, Title: "Monthly"}).Error)
	now := common.GetTimestamp()
	subs := []model.UserSubscription{
		{Id: 1, UserId: 101, PlanId: 1, Status: "active", EndTime: now + 7200, AmountTotal: 1000, AmountUsed: 300, NextResetTime: now - 10},
		{Id: 2, UserId: 101, PlanId: 999, Status: "active", EndTime: now + 10800, AmountTotal: 0, AmountUsed: 900},
		{Id: 3, UserId: 101, PlanId: 1, Status: "active", EndTime: now - 1},
		{Id: 4, UserId: 101, PlanId: 1, Status: "cancelled", EndTime: now + 7200},
		{Id: 5, UserId: 103, PlanId: 1, Status: "active", EndTime: now + 7200},
	}
	require.NoError(t, db.Create(&subs).Error)

	for _, test := range []struct {
		name    string
		url     string
		handler gin.HandlerFunc
	}{
		{"list", "/api/user/?p=1&page_size=2&sort_by=id&sort_order=asc", GetAllUsers},
		{"search", "/api/user/search?keyword=subscribed&p=1&page_size=2&sort_by=id&sort_order=asc", SearchUsers},
	} {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodGet, test.url, nil)
			test.handler(c)
			require.Equal(t, http.StatusOK, recorder.Code)
			var response struct {
				Success bool `json:"success"`
				Data    struct {
					Items []adminUserListItem `json:"items"`
					Total int                 `json:"total"`
				} `json:"data"`
			}
			require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
			require.True(t, response.Success, recorder.Body.String())
			require.Len(t, response.Data.Items, 2)
			assert.Equal(t, 101, response.Data.Items[0].Id)
			assert.Empty(t, response.Data.Items[0].Password)
			assert.Equal(t, []model.UserSubscriptionStatus{
				{Id: 1, PlanTitle: "Monthly", AmountTotal: 1000, AmountUsed: 300, EndTime: now + 7200, NextResetTime: now - 10},
				{Id: 2, PlanTitle: "", AmountTotal: 0, AmountUsed: 900, EndTime: now + 10800},
			}, response.Data.Items[0].Subscriptions)
			assert.NotNil(t, response.Data.Items[1].Subscriptions)
			assert.Empty(t, response.Data.Items[1].Subscriptions)
		})
	}
	var after model.UserSubscription
	require.NoError(t, db.First(&after, 1).Error)
	assert.EqualValues(t, 300, after.AmountUsed, "viewing the list must not reset overdue quota")
	assert.Equal(t, now-10, after.NextResetTime)

	statuses, err := model.GetUserSubscriptionStatuses([]int{102})
	require.NoError(t, err)
	assert.Empty(t, statuses, "do not include subscriptions from users outside the page")
}
