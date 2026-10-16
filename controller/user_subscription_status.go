package controller

import "github.com/QuantumNous/new-api/model"

type adminUserListItem struct {
	*model.User
	Subscriptions []model.UserSubscriptionStatus `json:"subscriptions"`
}

func usersWithSubscriptionStatus(users []*model.User) ([]adminUserListItem, error) {
	userIds := make([]int, 0, len(users))
	for _, user := range users {
		userIds = append(userIds, user.Id)
	}
	statuses, err := model.GetUserSubscriptionStatuses(userIds)
	if err != nil {
		return nil, err
	}
	items := make([]adminUserListItem, 0, len(users))
	for _, user := range users {
		subscriptions := statuses[user.Id]
		if subscriptions == nil {
			subscriptions = []model.UserSubscriptionStatus{}
		}
		items = append(items, adminUserListItem{User: user, Subscriptions: subscriptions})
	}
	return items, nil
}
