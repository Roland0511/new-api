package relay

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/gin-gonic/gin"
)

func DecisionsHelper(c *gin.Context, info *relaycommon.RelayInfo) *types.NewAPIError {
	info.InitChannelMeta(c)
	request, ok := info.Request.(dto.DecisionsRequest)
	if !ok {
		return types.NewError(errors.New("invalid decisions request"), types.ErrorCodeInvalidRequest, types.ErrOptionWithSkipRetry())
	}
	if info.RelayFormat == types.RelayFormatOpenAIDecisions && info.ChannelType != constant.ChannelTypeOpenAI ||
		info.RelayFormat == types.RelayFormatJEVDecisions && info.ChannelType != constant.ChannelTypeOpenRouter {
		return types.NewError(errors.New("channel does not support this decisions protocol"), types.ErrorCodeInvalidRequest)
	}
	// OpenRouter Decisions requires its own verified endpoint price.
	if info.ChannelType == constant.ChannelTypeOpenRouter {
		if _, configured := billing_setting.GetConfiguredEndpointBillingExpr(types.EndpointTypeJEVDecisions, info.OriginModelName); !configured {
			return types.NewError(errors.New("OpenRouter Decisions price is not configured"), types.ErrorCodeModelPriceError, types.ErrOptionWithSkipRetry())
		}
	}
	if err := helper.ModelMappedHelper(c, info, request); err != nil {
		return types.NewError(err, types.ErrorCodeChannelModelMappedError, types.ErrOptionWithSkipRetry())
	}
	effectiveRequest, body, apiErr := BuildDecisionsRequestBody(info, request)
	if apiErr != nil {
		return apiErr
	}
	reader, closer, err := relaycommon.NewOutboundJSONBody(body)
	if err != nil {
		return types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
	}
	defer closer.Close()
	adaptor := GetAdaptor(info.ApiType)
	if adaptor == nil {
		return types.NewError(fmt.Errorf("invalid api type: %d", info.ApiType), types.ErrorCodeInvalidApiType, types.ErrOptionWithSkipRetry())
	}
	adaptor.Init(info)
	response, err := adaptor.DoRequest(c, info, reader)
	if err != nil {
		return types.NewOpenAIError(err, types.ErrorCodeDoRequestFailed, http.StatusBadGateway)
	}
	httpResponse, ok := response.(*http.Response)
	if !ok || httpResponse == nil {
		return types.NewError(errors.New("invalid decisions HTTP response"), types.ErrorCodeBadResponse)
	}
	defer httpResponse.Body.Close()
	if httpResponse.StatusCode != http.StatusOK {
		apiErr := service.RelayErrorHandler(c.Request.Context(), httpResponse, false)
		service.ResetStatusCode(apiErr, c.GetString("status_code_mapping"))
		return apiErr
	}
	const maxDecisionsResponseBytes = 32 << 20
	responseBody, err := io.ReadAll(io.LimitReader(httpResponse.Body, maxDecisionsResponseBytes+1))
	if err != nil || len(responseBody) > maxDecisionsResponseBytes {
		return types.NewError(errors.New("invalid decisions response body"), types.ErrorCodeBadResponse)
	}
	usage, err := dto.ValidateDecisionsResponse(effectiveRequest, responseBody)
	if err != nil {
		return types.NewErrorWithStatusCode(err, types.ErrorCodeBadResponse, http.StatusBadGateway)
	}
	// Decisions must never settle an estimate after a failed expression run.
	// Existing text settlement can retain the reservation for other protocols.
	snapshot := info.TieredBillingSnapshot
	if snapshot == nil {
		return types.NewError(errors.New("missing Decisions price snapshot"), types.ErrorCodeModelPriceError, types.ErrOptionWithSkipRetry())
	}
	params := service.BuildTieredTokenParams(usage, false, billingexpr.UsedVarsByHash(snapshot.ExprString, snapshot.ExprHash))
	requestInput := billingexpr.RequestInput{}
	if info.BillingRequestInput != nil {
		requestInput = *info.BillingRequestInput
	}
	if _, err := billingexpr.ComputeTieredQuotaWithRequest(snapshot, params, requestInput); err != nil {
		return types.NewError(err, types.ErrorCodeModelPriceError, types.ErrOptionWithSkipRetry())
	}
	info.FinalRequestRelayFormat = info.RelayFormat
	if err := service.PostDecisionsConsumeQuota(c, info, usage); err != nil {
		return types.NewError(err, types.ErrorCodeUpdateDataError, types.ErrOptionWithSkipRetry())
	}
	c.Data(http.StatusOK, "application/json", responseBody)
	return nil
}

// BuildDecisionsRequestBody is shared by the relay and channel tests, so tests
// exercise the same mapping, override validation and privacy policy as calls.
func BuildDecisionsRequestBody(info *relaycommon.RelayInfo, request dto.DecisionsRequest) (dto.DecisionsRequest, []byte, *types.NewAPIError) {
	var fields map[string]json.RawMessage
	if err := common.Unmarshal(request.DecisionsBody(), &fields); err != nil {
		return nil, nil, types.NewError(err, types.ErrorCodeInvalidRequest, types.ErrOptionWithSkipRetry())
	}
	mappedModel, err := common.Marshal(info.UpstreamModelName)
	if err != nil {
		return nil, nil, types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
	}
	fields["model"] = mappedModel
	body, err := common.Marshal(fields)
	if err != nil {
		return nil, nil, types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
	}
	if len(info.ParamOverride) > 0 {
		body, err = relaycommon.ApplyParamOverrideWithRelayInfo(body, info)
		if err != nil {
			return nil, nil, newAPIErrorFromParamOverride(err)
		}
	}
	effectiveRequest, err := dto.ParseDecisionsRequest(body)
	if err != nil || effectiveRequest.DecisionsFormat() != info.RelayFormat {
		if err == nil {
			err = errors.New("parameter override cannot change decisions protocol")
		}
		return nil, nil, types.NewErrorWithStatusCode(err, types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
	}
	// Apply privacy filtering last so an override cannot bypass the channel's
	// existing safety_identifier policy. Keep all other native fields unchanged.
	if info.RelayFormat == types.RelayFormatOpenAIDecisions && !info.ChannelOtherSettings.AllowSafetyIdentifier {
		var filtered map[string]json.RawMessage
		if err := common.Unmarshal(body, &filtered); err != nil {
			return nil, nil, types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
		}
		delete(filtered, "safety_identifier")
		body, err = common.Marshal(filtered)
		if err != nil {
			return nil, nil, types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
		}
	}
	return effectiveRequest, body, nil
}
