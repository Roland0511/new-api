package billing_setting

import (
	"maps"
	"strings"

	"github.com/QuantumNous/new-api/relaykit/types"
)

func DecisionsEndpoint(format types.RelayFormat) types.EndpointType {
	switch format {
	case types.RelayFormatOpenAIDecisions:
		return types.EndpointTypeOpenAIDecisions
	case types.RelayFormatJEVDecisions:
		return types.EndpointTypeJEVDecisions
	default:
		return ""
	}
}

func EndpointBillingExprKey(endpoint types.EndpointType, model string) string {
	return string(endpoint) + "::" + model
}

func SplitEndpointBillingExprKey(key string) (types.EndpointType, string, bool) {
	endpoint, model, ok := strings.Cut(key, "::")
	valid := types.EndpointType(endpoint) == types.EndpointTypeOpenAIDecisions || types.EndpointType(endpoint) == types.EndpointTypeJEVDecisions
	return types.EndpointType(endpoint), model, ok && valid && strings.TrimSpace(model) != ""
}

func GetConfiguredEndpointBillingExpr(endpoint types.EndpointType, model string) (string, bool) {
	expr, ok := billingSetting.EndpointBillingExpr[EndpointBillingExprKey(endpoint, model)]
	return expr, ok
}

func GetEndpointBillingExpr(endpoint types.EndpointType, model string) (string, bool) {
	if expr, ok := GetConfiguredEndpointBillingExpr(endpoint, model); ok {
		return expr, true
	}
	expr, ok := builtinEndpointBillingExpr[EndpointBillingExprKey(endpoint, model)]
	return expr, ok
}

func GetEndpointBillingExprCopy() map[string]string {
	return maps.Clone(billingSetting.EndpointBillingExpr)
}
func GetBuiltinEndpointBillingExprCopy() map[string]string {
	return maps.Clone(builtinEndpointBillingExpr)
}
