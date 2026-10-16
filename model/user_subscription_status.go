package model

import "github.com/QuantumNous/new-api/common"

// UserSubscriptionStatus is the read-only quota summary exposed in admin user lists.
type UserSubscriptionStatus struct {
	Id            int    `json:"id"`
	UserId        int    `json:"-"`
	PlanTitle     string `json:"plan_title"`
	AmountTotal   int64  `json:"amount_total"`
	AmountUsed    int64  `json:"amount_used"`
	EndTime       int64  `json:"end_time"`
	NextResetTime int64  `json:"next_reset_time"`
}

// GetUserSubscriptionStatuses loads only the requested page's active subscriptions.
// Reading status must never reset quota or otherwise mutate billing state.
func GetUserSubscriptionStatuses(userIds []int) (map[int][]UserSubscriptionStatus, error) {
	result := make(map[int][]UserSubscriptionStatus, len(userIds))
	if len(userIds) == 0 {
		return result, nil
	}
	var statuses []UserSubscriptionStatus
	err := DB.Model(&UserSubscription{}).
		Select("user_subscriptions.id, user_subscriptions.user_id, COALESCE(subscription_plans.title, '') AS plan_title, user_subscriptions.amount_total, user_subscriptions.amount_used, user_subscriptions.end_time, user_subscriptions.next_reset_time").
		Joins("LEFT JOIN subscription_plans ON subscription_plans.id = user_subscriptions.plan_id").
		Where("user_subscriptions.user_id IN ? AND user_subscriptions.status = ? AND user_subscriptions.end_time > ?", userIds, "active", common.GetTimestamp()).
		Order("user_subscriptions.end_time ASC, user_subscriptions.id ASC").
		Scan(&statuses).Error
	if err != nil {
		return nil, err
	}
	for _, status := range statuses {
		result[status.UserId] = append(result[status.UserId], status)
	}
	return result, nil
}
