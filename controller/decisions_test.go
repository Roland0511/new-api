package controller

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/QuantumNous/new-api/relay"
	"github.com/QuantumNous/new-api/relay/channel/openai"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const openAIDecisionsBody = `{"model":"gpt-6-luna","input":"blue sky","questions":[{"type":"predicate","instructions":"Is the sky blue?"}]}`
const jevDecisionsBody = `{"model":"jev-latest","state":{"text":"blue sky","id":9007199254740993},"questions":{"blue":{"type":"noul","instructions":{"question":"Is the sky blue?"}}}}`
const openAIDecisionsResponse = `{"model":"gpt-6-luna","answers":[{"name":null,"type":"predicate","probability":0.9}],"usage":{"input_tokens":1000,"output_tokens":20,"total_tokens":1020,"input_tokens_details":{"cached_tokens":100,"cache_write_tokens":100},"output_tokens_details":{"reasoning_tokens":0}},"future_field":{"kept":true}}`
const jevDecisionsResponse = `{"model":"jev-1.13.0","answers":{"blue":{"type":"noul","noul":0.9}},"usage":{"input_tokens":1000,"output_tokens":20},"future_field":{"kept":true}}`

func TestDecisionsTypeSafeModelPreset(t *testing.T) {
	adaptor := &openai.Adaptor{}
	adaptor.Init(&relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeTypeSafe}})
	assert.Equal(t, []string{"jev-latest", "jev-preview", "jev-1.13.0"}, adaptor.GetModelList())
	assert.Equal(t, "typesafe", adaptor.GetChannelName())
	assert.Equal(t, []constant.EndpointType{constant.EndpointTypeJEVDecisions}, common.GetEndpointTypesByChannelType(constant.ChannelTypeTypeSafe, "jev-latest"))
}

func TestDecisionsNativeValidation(t *testing.T) {
	for _, body := range []string{openAIDecisionsBody, jevDecisionsBody,
		`{"model":"gpt-6-luna","input":[{"role":"user","content":[{"type":"input_text","text":"blue"},{"type":"input_image","image_url":"data:image/png;base64,YQ==","detail":"original"}]}],"questions":[{"type":"choice","instructions":"Choose","choices":[{"value":false},{"value":true}]}],"stream":false,"safety_identifier":null}`,
		`{"model":"jev-preview","state":[],"questions":{"q":{"type":"choice","instructions":[],"criteria":{"a":null,"b":{},"c":[]}}}}`,
	} {
		request, err := dto.ParseDecisionsRequest([]byte(body))
		require.NoError(t, err, body)
		assert.JSONEq(t, body, string(request.DecisionsBody()))
	}
	for _, body := range []string{`null`, `{}`, `{"model":"x","input":"x","state":"x","questions":[]}`,
		strings.Replace(openAIDecisionsBody, `"input":"blue sky"`, `"state":"blue sky"`, 1),
		strings.Replace(jevDecisionsBody, `"state":{`, `"input":{`, 1),
		strings.Replace(openAIDecisionsBody, `"model":`, `"stream":true,"model":`, 1),
		strings.Replace(openAIDecisionsBody, `"input":"blue sky"`, `"input":[{"role":"assistant","content":"x"}]`, 1),
		`{"model":"x","input":[{"role":"user","content":[{"type":"input_image","image_url":"https://example.com/a.png"}]}],"questions":[{"type":"predicate","instructions":"x"}]}`,
		`{"model":"x","input":"x","questions":[{"type":"choice","instructions":"x","choices":[{"value":false},{"value":false}]}]}`,
		`{"model":"x","state":true,"questions":{"q":{"type":"noul","instructions":"x"}}}`,
		`{"model":"x","state":"x","questions":{"q":{"type":"score","instructions":"x","criteria":["one"]}}}`,
	} {
		_, err := dto.ParseDecisionsRequest([]byte(body))
		assert.Error(t, err, body)
	}

	request, err := dto.ParseDecisionsRequest([]byte(openAIDecisionsBody))
	require.NoError(t, err)
	usage, err := dto.ValidateDecisionsResponse(request, []byte(openAIDecisionsResponse))
	require.NoError(t, err)
	assert.Equal(t, 1000, usage.PromptTokens)
	assert.Equal(t, 100, usage.PromptTokensDetails.CachedTokens)
	for _, body := range []string{
		strings.Replace(openAIDecisionsResponse, `"input_tokens":1000`, `"input_tokens":-1`, 1),
		strings.Replace(openAIDecisionsResponse, `"input_tokens":1000`, `"input_tokens":1.5`, 1),
		strings.Replace(openAIDecisionsResponse, `"input_tokens":1000`, `"input_tokens":2147483648`, 1),
		strings.Replace(openAIDecisionsResponse, `"total_tokens":1020`, `"total_tokens":10`, 1),
		strings.Replace(openAIDecisionsResponse, `"cached_tokens":100`, `"cached_tokens":901`, 1),
		strings.Replace(openAIDecisionsResponse, `"probability":0.9`, `"probability":null`, 1),
		strings.Replace(openAIDecisionsResponse, `"name":null,`, ``, 1),
		strings.Replace(openAIDecisionsResponse, `"usage":`, `"ignored_usage":`, 1),
	} {
		_, err := dto.ValidateDecisionsResponse(request, []byte(body))
		assert.Error(t, err, body)
	}
	_, err = dto.ValidateDecisionsResponse(request, []byte(strings.Replace(openAIDecisionsResponse, `"type":"predicate","probability":0.9`, `"type":"refusal"`, 1)))
	assert.NoError(t, err)

	choice, err := dto.ParseDecisionsRequest([]byte(`{"model":"x","input":"x","questions":[{"type":"choice","instructions":"choose","choices":[{"value":false},{"value":true}]}]}`))
	require.NoError(t, err)
	choiceResponse := strings.Replace(openAIDecisionsResponse, `"type":"predicate","probability":0.9`, `"type":"choice","choice":false,"confidence":0.9,"probabilities":[{"value":false,"probability":0.9},{"value":true,"probability":0.1}]`, 1)
	_, err = dto.ValidateDecisionsResponse(choice, []byte(choiceResponse))
	assert.NoError(t, err)
	assert.NotPanics(t, func() {
		_, err = dto.ValidateDecisionsResponse(choice, []byte(strings.Replace(choiceResponse, `"choice":false`, `"choice":{}`, 1)))
		assert.Error(t, err)
	})
	jev, err := dto.ParseDecisionsRequest([]byte(`{"model":"jev-latest","state":"x","questions":{"s":{"type":"score","instructions":"rate","criteria":["low","high"]}}}`))
	require.NoError(t, err)
	_, err = dto.ValidateDecisionsResponse(jev, []byte(`{"model":"jev-latest","answers":{"s":{"type":"score","score":0.4,"confidence":0.8,"legend":{"0":"low","1":"high"},"probabilities":{"0":0.6,"1":0.4}}},"usage":{"input_tokens":123,"output_tokens":9}}`))
	assert.NoError(t, err, "JEV score is probability-weighted, not an integer")
}

func TestDecisionsTransportAndPrivacy(t *testing.T) {
	for _, tc := range []struct {
		channel        int
		base, expected string
	}{
		{constant.ChannelTypeOpenAI, "https://api.openai.com", "https://api.openai.com/v1/decisions"},
		{constant.ChannelTypeTypeSafe, "https://api.typesafe.ai", "https://api.typesafe.ai/v1/systemone"},
		{constant.ChannelTypeOpenRouter, "https://openrouter.ai/api", "https://openrouter.ai/api/alpha/decisions"},
	} {
		adaptor := &openai.Adaptor{}
		url, err := adaptor.GetRequestURL(&relaycommon.RelayInfo{RelayMode: relayconstant.RelayModeDecisions, ChannelMeta: &relaycommon.ChannelMeta{ChannelType: tc.channel, ChannelBaseUrl: tc.base}})
		require.NoError(t, err)
		assert.Equal(t, tc.expected, url)
	}
	request, err := dto.ParseDecisionsRequest([]byte(strings.Replace(openAIDecisionsBody, `"model":`, `"safety_identifier":"private","future_zero":0,"model":`, 1)))
	require.NoError(t, err)
	info := &relaycommon.RelayInfo{RelayFormat: types.RelayFormatOpenAIDecisions, ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "mapped-model"}}
	_, body, apiErr := relay.BuildDecisionsRequestBody(info, request)
	require.Nil(t, apiErr)
	assert.NotContains(t, string(body), "safety_identifier")
	assert.Contains(t, string(body), `"future_zero":0`)
	assert.Contains(t, string(body), `"model":"mapped-model"`)
	info.ChannelOtherSettings.AllowSafetyIdentifier = true
	_, body, apiErr = relay.BuildDecisionsRequestBody(info, request)
	require.Nil(t, apiErr)
	assert.Contains(t, string(body), `"safety_identifier":"private"`)
	info.ParamOverride = map[string]any{"stream": true}
	_, _, apiErr = relay.BuildDecisionsRequestBody(info, request)
	require.NotNil(t, apiErr)
	assert.Equal(t, http.StatusBadRequest, apiErr.StatusCode)
	jev, err := dto.ParseDecisionsRequest([]byte(jevDecisionsBody))
	require.NoError(t, err)
	info.RelayFormat, info.ParamOverride = types.RelayFormatJEVDecisions, nil
	_, body, apiErr = relay.BuildDecisionsRequestBody(info, jev)
	require.Nil(t, apiErr)
	assert.Contains(t, string(body), `9007199254740993`, "structured state must not lose integer precision")
}

func TestDecisionsEndpointPriceIsolation(t *testing.T) {
	withTieredBillingConfig(t, map[string]string{"gpt-6-luna": "tiered_expr"}, map[string]string{"gpt-6-luna": `p * 99 + c * 99`})
	previous := config.GlobalConfig.ExportAllConfigs()[billing_setting.EndpointBillingExprOption]
	config.UpdateConfigFromMap(config.GlobalConfig.Get("billing_setting"), map[string]string{"endpoint_billing_expr": "{}"})
	t.Cleanup(func() {
		config.UpdateConfigFromMap(config.GlobalConfig.Get("billing_setting"), map[string]string{"endpoint_billing_expr": previous})
	})
	for _, tc := range []struct {
		input, cached, written int
		cost                   float64
	}{
		{272000, 1000, 1000, 27000}, {272001, 1000, 1000, 54000.2}, {1000, 1000, 0, 0},
	} {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/decisions", strings.NewReader(openAIDecisionsBody))
		info := &relaycommon.RelayInfo{RelayFormat: types.RelayFormatOpenAIDecisions, OriginModelName: "gpt-6-luna", UserGroup: "default", UsingGroup: "default"}
		_, err := helper.ModelPriceHelper(c, info, 100, &types.TokenCountMeta{})
		require.NoError(t, err)
		assert.NotContains(t, info.TieredBillingSnapshot.ExprString, "99")
		params := service.BuildTieredTokenParams(&dto.Usage{PromptTokens: tc.input, CompletionTokens: 999, PromptTokensDetails: dto.InputTokenDetails{CachedTokens: tc.cached, CacheWriteTokens: tc.written}}, false, billingexpr.UsedVarsByHash(info.TieredBillingSnapshot.ExprString, info.TieredBillingSnapshot.ExprHash))
		cost, _, err := billingexpr.RunExprByHash(info.TieredBillingSnapshot.ExprString, info.TieredBillingSnapshot.ExprHash, params)
		require.NoError(t, err)
		assert.InDelta(t, tc.cost, cost, 0.0001)
	}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/decisions", strings.NewReader(openAIDecisionsBody))
	_, err := helper.ModelPriceHelper(c, &relaycommon.RelayInfo{RelayFormat: types.RelayFormatOpenAIDecisions, OriginModelName: "unpriced", UserGroup: "default"}, 100, &types.TokenCountMeta{})
	assert.Error(t, err, "unknown endpoint prices must not inherit or silently become free")
}

func TestDecisionsRelayDatabaseMatrix(t *testing.T) {
	require.NoError(t, i18n.Init())
	service.InitHttpClient()
	service.InitTokenEncoders()
	for _, dialect := range []struct{ kind, env string }{{"sqlite", ""}, {"mysql", "TEST_MYSQL_DSN"}, {"postgres", "TEST_POSTGRES_DSN"}} {
		t.Run(dialect.kind, func(t *testing.T) {
			dsn := os.Getenv(dialect.env)
			if dialect.env != "" && dsn == "" {
				t.Skipf("%s not configured", dialect.env)
			}
			db := modelManagementDB(t, dialect.kind, dsn)
			require.NoError(t, db.AutoMigrate(&model.Token{}, &model.Log{}, &model.UserSubscription{}))
			oldBatch, oldLog, oldCount, oldRetry := common.BatchUpdateEnabled, common.LogConsumeEnabled, constant.CountToken, common.RetryTimes
			common.BatchUpdateEnabled, common.LogConsumeEnabled, constant.CountToken, common.RetryTimes = false, true, true, 0
			oldQuota := *operation_setting.GetQuotaSetting()
			operation_setting.GetQuotaSetting().TrustQuotaUSD = 0
			oldGroups, _ := common.Marshal(ratio_setting.GetGroupRatioCopy())
			require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1}`))
			previous := config.GlobalConfig.ExportAllConfigs()[billing_setting.EndpointBillingExprOption]
			config.UpdateConfigFromMap(config.GlobalConfig.Get("billing_setting"), map[string]string{"endpoint_billing_expr": "{}"})
			t.Cleanup(func() {
				common.BatchUpdateEnabled, common.LogConsumeEnabled, constant.CountToken, common.RetryTimes = oldBatch, oldLog, oldCount, oldRetry
				*operation_setting.GetQuotaSetting() = oldQuota
				require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(string(oldGroups)))
				config.UpdateConfigFromMap(config.GlobalConfig.Get("billing_setting"), map[string]string{"endpoint_billing_expr": previous})
			})
			snapshot, err := model.GetModelPricingSnapshot([]string{"gpt-6-luna"})
			require.NoError(t, err)
			entry := snapshot.Entries[0]
			endpointDraft := model.PricingValues{billing_setting.EndpointBillingExprOption: map[string]any{"openai-decisions": "p * 0.3 + c * 0 + cr * 0 + cc * 0"}}
			require.NoError(t, model.UpdateModelPricing([]model.ModelPricingChange{{ModelName: entry.ModelName, ExpectedVersion: entry.Version, Pricing: endpointDraft}}))
			assert.ErrorIs(t, model.UpdateModelPricing([]model.ModelPricingChange{{ModelName: entry.ModelName, ExpectedVersion: entry.Version, Pricing: endpointDraft}}), model.ErrModelPricingConflict)
			snapshot, err = model.GetModelPricingSnapshot([]string{"gpt-6-luna"})
			require.NoError(t, err)
			require.NoError(t, model.UpdateModelPricing([]model.ModelPricingChange{{ModelName: "gpt-6-luna", ExpectedVersion: snapshot.Entries[0].Version, Pricing: model.PricingValues{"ModelRatio": 3.0}}}))
			snapshot, err = model.GetModelPricingSnapshot([]string{"gpt-6-luna"})
			require.NoError(t, err)
			assert.NotEmpty(t, snapshot.Entries[0].Configured[billing_setting.EndpointBillingExprOption], "older clients must preserve endpoint overrides")
			require.NoError(t, model.UpdateModelPricing([]model.ModelPricingChange{{ModelName: "gpt-6-luna", ExpectedVersion: snapshot.Entries[0].Version, Reset: true}}))
			for index, tc := range []struct {
				name, body, response, path string
				channel, expected, charge  int
			}{
				{"openai", openAIDecisionsBody, openAIDecisionsResponse, "/v1/decisions", constant.ChannelTypeOpenAI, 200, 40},
				{"refusal", openAIDecisionsBody, strings.Replace(openAIDecisionsResponse, `"type":"predicate","probability":0.9`, `"type":"refusal"`, 1), "/v1/decisions", constant.ChannelTypeOpenAI, 200, 40},
				{"typesafe", jevDecisionsBody, jevDecisionsResponse, "/v1/systemone", constant.ChannelTypeTypeSafe, 200, 21},
				{"retry", openAIDecisionsBody, openAIDecisionsResponse, "/v1/decisions", constant.ChannelTypeOpenAI, 200, 40},
				{"mapping", strings.Replace(openAIDecisionsBody, `gpt-6-luna`, `client-alias`, 1), openAIDecisionsResponse, "/v1/decisions", constant.ChannelTypeOpenAI, 200, 80},
				{"permission", openAIDecisionsBody, openAIDecisionsResponse, "", constant.ChannelTypeOpenAI, 403, 0},
				{"openrouter priced", jevDecisionsBody, jevDecisionsResponse, "/api/alpha/decisions", constant.ChannelTypeOpenRouter, 200, 50},
				{"bad usage", openAIDecisionsBody, strings.Replace(openAIDecisionsResponse, `"input_tokens":1000`, `"input_tokens":-1`, 1), "/v1/decisions", constant.ChannelTypeOpenAI, 502, 0},
				{"bad answer", openAIDecisionsBody, strings.Replace(openAIDecisionsResponse, `"probability":0.9`, `"probability":2`, 1), "/v1/decisions", constant.ChannelTypeOpenAI, 502, 0},
				{"stream", strings.Replace(openAIDecisionsBody, `"model":`, `"stream":true,"model":`, 1), openAIDecisionsResponse, "", constant.ChannelTypeOpenAI, 400, 0},
				{"azure excluded", openAIDecisionsBody, openAIDecisionsResponse, "", constant.ChannelTypeAzure, 503, 0},
				{"jev cannot use openai", jevDecisionsBody, jevDecisionsResponse, "", constant.ChannelTypeOpenAI, 503, 0},
				{"openrouter unpriced", jevDecisionsBody, jevDecisionsResponse, "", constant.ChannelTypeOpenRouter, 503, 0},
			} {
				t.Run(tc.name, func(t *testing.T) {
					common.RetryTimes = 0
					config.UpdateConfigFromMap(config.GlobalConfig.Get("billing_setting"), map[string]string{"endpoint_billing_expr": "{}"})
					if tc.name == "retry" {
						common.RetryTimes = 1
					}
					if tc.name == "mapping" {
						config.UpdateConfigFromMap(config.GlobalConfig.Get("billing_setting"), map[string]string{"endpoint_billing_expr": `{"openai-decisions::client-alias":"p * 0.2 + c * 0 + cr * 0 + cc * 0"}`})
					}
					if tc.name == "openrouter priced" {
						config.UpdateConfigFromMap(config.GlobalConfig.Get("billing_setting"), map[string]string{"endpoint_billing_expr": `{"jev-decisions::jev-latest":"p * 0.1 + c * 0"}`})
					}
					var calls atomic.Int32
					server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						attempt := calls.Add(1)
						assert.Equal(t, tc.path, r.URL.Path)
						assert.Equal(t, "Bearer test-only-key", r.Header.Get("Authorization"))
						body, err := io.ReadAll(r.Body)
						require.NoError(t, err)
						if tc.channel == constant.ChannelTypeTypeSafe {
							assert.Contains(t, string(body), "9007199254740993")
						}
						w.Header().Set("Content-Type", "application/json")
						if tc.name == "mapping" {
							assert.Contains(t, string(body), `"model":"gpt-6-luna"`)
						}
						if tc.name == "retry" && attempt == 1 {
							w.WriteHeader(503)
							_, _ = io.WriteString(w, `{"error":{"message":"temporary upstream outage"}}`)
							return
						}
						_, _ = io.WriteString(w, tc.response)
					}))
					defer server.Close()
					const initial = 1000000
					user := model.User{Username: fmt.Sprintf("decisions_%d", index), AffCode: fmt.Sprintf("dc_%d", index), Group: "default", Quota: initial, Status: common.UserStatusEnabled}
					require.NoError(t, db.Create(&user).Error)
					token := model.Token{UserId: user.Id, Key: fmt.Sprintf("test-only-token-%d", index), RemainQuota: initial, Status: common.TokenStatusEnabled}
					require.NoError(t, db.Create(&token).Error)
					require.NoError(t, db.Model(&model.Ability{}).Where("1=1").Update("enabled", false).Error)
					base := server.URL
					if tc.channel == constant.ChannelTypeOpenRouter {
						base += "/api"
					}
					channel := model.Channel{Name: tc.name, Type: tc.channel, Key: "test-only-key", BaseURL: common.GetPointer(base), Status: common.ChannelStatusEnabled, Group: "default", Models: "gpt-6-luna,jev-latest,client-alias", AutoBan: common.GetPointer(0)}
					if tc.name == "mapping" {
						channel.ModelMapping = common.GetPointer(`{"client-alias":"gpt-6-luna"}`)
					}
					require.NoError(t, db.Create(&channel).Error)
					require.NoError(t, channel.AddAbilities(db))
					cache, err := model.GetUserCache(user.Id)
					require.NoError(t, err)
					router := gin.New()
					router.POST("/v1/decisions", func(c *gin.Context) {
						cache.WriteContext(c)
						common.SetContextKey(c, constant.ContextKeyUserId, user.Id)
						common.SetContextKey(c, constant.ContextKeyTokenId, token.Id)
						common.SetContextKey(c, constant.ContextKeyTokenKey, token.Key)
						common.SetContextKey(c, constant.ContextKeyTokenGroup, "default")
						common.SetContextKey(c, constant.ContextKeyUsingGroup, "default")
						if tc.name == "permission" {
							common.SetContextKey(c, constant.ContextKeyTokenModelLimitEnabled, true)
							common.SetContextKey(c, constant.ContextKeyTokenModelLimit, map[string]bool{"not-this-model": true})
						}
						c.Next()
					}, middleware.Distribute(), func(c *gin.Context) { Relay(c, types.RelayFormat(c.GetString("decisions_format"))) })
					w := httptest.NewRecorder()
					req := httptest.NewRequest(http.MethodPost, "/v1/decisions", strings.NewReader(tc.body))
					req.Header.Set("Content-Type", "application/json")
					router.ServeHTTP(w, req)
					assert.Equal(t, tc.expected, w.Code, w.Body.String())
					if tc.expected >= 500 {
						require.Eventually(t, func() bool {
							var actualUser model.User
							var actualToken model.Token
							return db.First(&actualUser, user.Id).Error == nil && db.First(&actualToken, token.Id).Error == nil && actualUser.Quota == initial && actualToken.RemainQuota == initial
						}, time.Second, time.Millisecond, "failed requests must refund their reservation")
					}
					require.NoError(t, db.First(&user, user.Id).Error)
					require.NoError(t, db.First(&token, token.Id).Error)
					require.NoError(t, db.First(&channel, channel.Id).Error)
					assert.Equal(t, initial-tc.charge, user.Quota)
					assert.Equal(t, initial-tc.charge, token.RemainQuota)
					assert.EqualValues(t, tc.charge, channel.UsedQuota)
					var logs int64
					require.NoError(t, db.Model(&model.Log{}).Where("user_id=? AND type=?", user.Id, model.LogTypeConsume).Count(&logs).Error)
					if tc.expected == 200 {
						wantCalls := 1
						if tc.name == "retry" {
							wantCalls = 2
						}
						assert.EqualValues(t, wantCalls, calls.Load())
						assert.EqualValues(t, 1, logs)
						assert.JSONEq(t, tc.response, w.Body.String())
					} else {
						assert.EqualValues(t, 0, logs)
						if tc.expected != 502 {
							assert.EqualValues(t, 0, calls.Load())
						}
					}
				})
			}
		})
	}
}
