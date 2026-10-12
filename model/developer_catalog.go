package model

import (
	"math"
	"slices"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	relaydto "github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/expr-lang/expr/ast"
	"github.com/expr-lang/expr/parser"
	"github.com/shopspring/decimal"
)

// DeveloperCatalogEntry keeps local routing state separate from the public DTO.
// It is loaded in four batched reads, independently of the model count and the
// optional channel cache. Channel credentials are never selected.
type DeveloperCatalogEntry struct {
	Name       string
	metadata   *Model
	vendorName string
	routes     []developerCatalogRoute
}

type developerCatalogRoute struct {
	group   string
	channel *Channel
	setting relaydto.ChannelSettings
	other   relaydto.ChannelOtherSettings
	mapped  string
}

func GetDeveloperCatalogEntries(groups []string) ([]DeveloperCatalogEntry, error) {
	entries := make([]DeveloperCatalogEntry, 0)
	if len(groups) == 0 {
		return entries, nil
	}
	connections, err := GetModelConnections()
	if err != nil {
		return nil, err
	}
	ids := make([]int, 0)
	seenIDs := make(map[int]bool)
	names := make([]string, 0)
	seenNames := make(map[string]bool)
	for _, connection := range connections {
		if !slices.Contains(groups, connection.Group) {
			continue
		}
		if !seenIDs[connection.ChannelId] {
			ids = append(ids, connection.ChannelId)
			seenIDs[connection.ChannelId] = true
		}
		if !seenNames[connection.Model] {
			names = append(names, connection.Model)
			seenNames[connection.Model] = true
		}
	}
	if len(ids) == 0 {
		return entries, nil
	}
	var channels []Channel
	if err := DB.Select("id", "type", "status", "models", "model_mapping", "setting", "settings", "base_url").Where("id IN ? AND status = ?", ids, common.ChannelStatusEnabled).Find(&channels).Error; err != nil {
		return nil, err
	}
	var metadata []Model
	if err := DB.Find(&metadata).Error; err != nil {
		return nil, err
	}
	var vendors []Vendor
	if err := DB.Select("id", "name").Find(&vendors).Error; err != nil {
		return nil, err
	}
	metaByName := resolveModelMetadata(metadata, names)
	vendorByID := make(map[int]string, len(vendors))
	for _, vendor := range vendors {
		vendorByID[vendor.Id] = vendor.Name
	}
	channelByID := make(map[int]*Channel, len(channels))
	settings := make(map[int]relaydto.ChannelSettings, len(channels))
	others := make(map[int]relaydto.ChannelOtherSettings, len(channels))
	mappings := make(map[int]map[string]string, len(channels))
	for i := range channels {
		channel := &channels[i]
		// Accessors may persist repairs on malformed settings. Validate and
		// decode here so this endpoint is strictly read-only even in that case.
		var setting relaydto.ChannelSettings
		if channel.Setting != nil && strings.TrimSpace(*channel.Setting) != "" {
			if err := common.UnmarshalJsonStr(*channel.Setting, &setting); err != nil {
				continue
			}
		}
		var other relaydto.ChannelOtherSettings
		if strings.TrimSpace(channel.OtherSettings) != "" {
			if err := common.UnmarshalJsonStr(channel.OtherSettings, &other); err != nil {
				continue
			}
		}
		if preset := common.GetAdvancedCustomPreset(channel.Type); preset != nil {
			other.AdvancedCustom = preset
		}
		mapping := make(map[string]string)
		if channel.ModelMapping != nil && strings.TrimSpace(*channel.ModelMapping) != "" {
			if err := common.UnmarshalJsonStr(*channel.ModelMapping, &mapping); err != nil {
				continue
			}
		}
		channelByID[channel.Id], settings[channel.Id], others[channel.Id], mappings[channel.Id] = channel, setting, other, mapping
	}
	byName := make(map[string]*DeveloperCatalogEntry, len(names))
	for _, connection := range connections {
		channel := channelByID[connection.ChannelId]
		if channel == nil || !slices.Contains(groups, connection.Group) || !slices.Contains(channel.GetModels(), connection.Model) {
			continue
		}
		meta := metaByName[connection.Model]
		if meta != nil && meta.Status != 1 {
			continue
		}
		mapped, cyclic := followChannelModelMapping(mappings[channel.Id], connection.Model)
		if cyclic {
			continue
		}
		entry := byName[connection.Model]
		if entry == nil {
			entry = &DeveloperCatalogEntry{Name: connection.Model, metadata: meta}
			if meta != nil {
				entry.vendorName = vendorByID[meta.VendorID]
			}
			byName[connection.Model] = entry
		}
		entry.routes = append(entry.routes, developerCatalogRoute{group: connection.Group, channel: channel, setting: settings[channel.Id], other: others[channel.Id], mapped: mapped})
	}
	slices.Sort(names)
	for _, name := range names {
		if entry := byName[name]; entry != nil {
			entries = append(entries, *entry)
		}
	}
	return entries, nil
}

// PublicModel projects only prices and endpoints which have a reachable local
// route. Different route/group prices are deliberately marked variable.
func (entry DeveloperCatalogEntry) PublicModel(billingName string, groups map[string]decimal.Decimal, currencyRate decimal.Decimal, acceptUnset bool) (dto.DeveloperCatalogModel, bool) {
	result := dto.DeveloperCatalogModel{ID: entry.Name, Endpoints: make([]dto.DeveloperCatalogEndpoint, 0)}
	// These aliases normalize persisted vendor metadata, not model-name guesses.
	providerNames := map[string]string{"openai": "OpenAI", "anthropic": "Anthropic", "google": "Google", "gemini": "Google", "deepseek": "DeepSeek", "minimax": "MiniMax", "moonshot": "Moonshot", "cohere": "Cohere", "cloudflare": "Cloudflare", "jina": "Jina", "mistral": "Mistral", "xai": "xAI", "meta": "Meta", "microsoft": "Microsoft", "微软": "Microsoft", "腾讯": "Tencent", "讯飞": "iFlytek", "百度": "Baidu", "阿里巴巴": "Alibaba", "智谱": "Zhipu", "字节跳动": "ByteDance", "零一万物": "Yi", "快手": "Kuaishou", "即梦": "Jimeng", "vidu": "Vidu"}
	result.Provider = providerNames[strings.ToLower(strings.TrimSpace(entry.vendorName))]
	if result.Provider == "" {
		result.Provider = strings.TrimSpace(entry.vendorName)
	}
	generation := jsplugin.DefaultRegistry.Generation()
	var initialPricing []byte
	categories := make(map[string]bool)
	declaredChat := false
	if entry.metadata != nil && strings.TrimSpace(entry.metadata.Endpoints) != "" {
		var declared map[string]any
		if common.UnmarshalJsonStr(entry.metadata.Endpoints, &declared) == nil {
			for _, endpoint := range []constant.EndpointType{constant.EndpointTypeOpenAI, constant.EndpointTypeOpenAIResponse, constant.EndpointTypeAnthropic, constant.EndpointTypeGemini} {
				if _, exists := declared[string(endpoint)]; exists {
					declaredChat = true
				}
			}
			for key := range declared {
				if strings.HasPrefix(key, "audio") || key == "openai-video" || key == "image-generation" || key == "embeddings" || key == "jina-rerank" {
					declaredChat = false
					break
				}
			}
		}
	}
	for _, route := range entry.routes {
		groupRate, allowed := groups[route.group]
		if !allowed || groupRate.IsNegative() {
			continue
		}
		endpoints := common.GetEndpointTypesByChannelType(route.channel.Type, entry.Name)
		if constant.IsAdvancedCustomChannel(route.channel.Type) {
			if route.other.AdvancedCustom == nil {
				continue
			}
			endpoints = route.other.AdvancedCustom.SupportedEndpointTypesForModel(entry.Name)
		}
		if route.channel.Type == constant.ChannelTypeTaskPlugin {
			endpoints = nil
		}
		boundTaskModel := false
		providers := generation.PluginsByModel(entry.Name)
		if len(providers) == 0 {
			providers = generation.PluginsByModel(route.mapped)
		}
		for _, provider := range providers {
			if entry.routeBindsPlugin(route, provider, generation) {
				boundTaskModel = true
				break
			}
		}
		if boundTaskModel {
			// A media driver owning the model/channel is not evidence that its
			// generic channel type can also relay this model as chat.
			endpoints = nil
		}
		paths := make(map[string]string)
		for _, endpoint := range endpoints {
			if info, ok := common.GetDefaultEndpointInfo(endpoint); ok {
				paths[info.Path] = info.Method
			} else if endpoint == constant.EndpointTypeOpenAIVideo {
				paths["/v1/videos"] = "POST"
			}
		}
		// Model-scoped protocol candidates and the selected channel binding
		// determine the executing plugin. Installed plugins alone do not.
		for _, protocol := range jsplugin.HostProtocols() {
			for _, operation := range protocol.Operations {
				if operation.ModelField != "" {
					paths[operation.Path] = "POST"
				}
			}
		}
		for _, binding := range generation.Routes() {
			if binding.Route.Type == jsplugin.RouteTypeQuery || !entry.routeBindsPlugin(route, binding.Plugin, generation) {
				continue
			}
			if !slices.Contains(binding.Plugin.Meta.Models, route.mapped) && !slices.Contains(binding.Plugin.Meta.Models, entry.Name) {
				continue
			}
			if len(binding.Route.Models) > 0 && !slices.Contains(binding.Route.Models, route.mapped) && !slices.Contains(binding.Route.Models, entry.Name) {
				continue
			}
			paths[binding.Route.Path] = binding.Route.Method
		}
		for path, method := range paths {
			var plugin *jsplugin.LoadedPlugin
			candidates := generation.LookupEndpointCandidates(method, path, entry.Name)
			if len(candidates) == 0 {
				candidates = generation.LookupEndpointCandidates(method, path, route.mapped)
			}
			for _, candidate := range candidates {
				if entry.routeBindsPlugin(route, candidate.Plugin, generation) {
					plugin = candidate.Plugin
					break
				}
			}
			if binding, exists := generation.LookupDeclaredRoute(method, path); exists && entry.routeBindsPlugin(route, binding.Plugin, generation) {
				plugin = binding.Plugin
			}
			if plugin == nil {
				// Host protocol enumeration above is only a candidate set.
				// Retain unclaimed paths only when native routing supports them.
				if !slices.ContainsFunc(endpoints, func(endpoint constant.EndpointType) bool {
					info, ok := common.GetDefaultEndpointInfo(endpoint)
					return ok && info.Path == path || endpoint == constant.EndpointTypeOpenAIVideo && path == "/v1/videos"
				}) {
					continue
				}
			}
			pricing, billing, configured := catalogRoutePricing(entry.Name, billingName, route.mapped, path, route.channel.Type, plugin, groupRate.Mul(currencyRate), acceptUnset)
			if !configured {
				continue
			}
			encoded, _ := common.Marshal(struct {
				Pricing dto.DeveloperCatalogPricing
				Billing dto.DeveloperCatalogBilling
			}{pricing, billing})
			if initialPricing == nil {
				initialPricing, result.Pricing, result.Billing = encoded, pricing, billing
			} else if string(initialPricing) != string(encoded) {
				result.Pricing = dto.DeveloperCatalogPricing{Variable: true}
				features := append(slices.Clone(result.Billing.Features), billing.Features...)
				slices.Sort(features)
				result.Billing = dto.DeveloperCatalogBilling{Mode: "variable", Features: slices.Compact(features)}
			}
			endpoint := dto.DeveloperCatalogEndpoint{Path: path, Method: method}
			if !slices.Contains(result.Endpoints, endpoint) {
				result.Endpoints = append(result.Endpoints, endpoint)
			}
			switch path {
			case "/v1/images/generations", "/v1/images/edits":
				categories["image"] = true
			case "/v1/videos", "/v1/videos/generations":
				categories["video"] = true
			case "/v1/audio/speech", "/v1/audio/transcriptions", "/v1/audio/translations":
				categories["audio"] = true
			case "/v1/embeddings":
				categories["embedding"] = true
			case "/v1/rerank":
				categories["rerank"] = true
			case "/v1/chat/completions", "/v1/messages", "/v1/responses", "/v1/responses/compact", "/v1beta/models/{model}:generateContent":
				if plugin == nil && (declaredChat || constant.IsAdvancedCustomChannel(route.channel.Type)) {
					categories["chat"] = true
				}
			}
		}
	}
	if len(result.Endpoints) == 0 {
		return result, false
	}
	// Dedicated media endpoints are stronger evidence than generic chat
	// conversion support; conflicting dedicated categories remain unknown.
	chat := categories["chat"]
	delete(categories, "chat")
	if len(categories) == 1 {
		for category := range categories {
			result.Category = category
		}
	} else if len(categories) == 0 && chat {
		result.Category = "chat"
	}
	slices.SortFunc(result.Endpoints, func(a, b dto.DeveloperCatalogEndpoint) int {
		if a.Path != b.Path {
			return strings.Compare(a.Path, b.Path)
		}
		return strings.Compare(a.Method, b.Method)
	})
	return result, true
}

func (entry DeveloperCatalogEntry) routeBindsPlugin(route developerCatalogRoute, plugin *jsplugin.LoadedPlugin, generation *jsplugin.RoutingGeneration) bool {
	if plugin == nil {
		return false
	}
	if route.channel.Type == constant.ChannelTypeTaskPlugin || route.channel.Type == constant.ChannelTypeNewAPI {
		return route.channel.BindsTaskPluginForModel(plugin.Meta.Key, entry.Name, generation)
	}
	if route.channel.Type == constant.ChannelTypeOpenAI && plugin.Meta.Key == "dflop-tts" {
		return route.setting.BindsTaskPlugin(plugin.Meta.Key)
	}
	return slices.Contains(plugin.Meta.ChannelTypes, route.channel.Type)
}

func catalogRoutePricing(name, billingName, mapped, path string, channelType int, plugin *jsplugin.LoadedPlugin, multiplier decimal.Decimal, acceptUnset bool) (dto.DeveloperCatalogPricing, dto.DeveloperCatalogBilling, bool) {
	pricing := dto.DeveloperCatalogPricing{}
	billing := dto.DeveloperCatalogBilling{Mode: "token"}
	if plugin != nil {
		requireQuantity := plugin.Meta.Key == "dflop-media" || plugin.Meta.Key == "dflop-image" || plugin.Meta.Key == "dflop-tts"
		plan := billing_setting.ResolveTaskBillingPlan(plugin.Meta.Key, name, mapped, plugin, requireQuantity)
		if plan.Reason != "" {
			return pricing, billing, false
		}
		expression, exists := plan.Expression, plan.ExpressionFound
		schema, _ := plugin.Meta.UsageForModels(mapped, name)
		if exists || billing_setting.GetBillingMode(name) == billing_setting.BillingModeTieredExpr {
			if !exists || !billing_setting.TaskExprCompatible(expression, schema) {
				return pricing, billing, false
			}
			pricing, billing = catalogExpressionPricing(expression, schema, multiplier)
			return pricing, billing, true
		}
		// Legacy task hooks can apply provider-specific usage multipliers that
		// cannot be recovered from a per-call base price without a request.
		_, priced := ratio_setting.GetModelPrice(name, false)
		_, ratioConfigured, _ := ratio_setting.GetModelRatio(name)
		if !priced && !ratioConfigured && !acceptUnset {
			return pricing, billing, false
		}
		return dto.DeveloperCatalogPricing{Variable: true}, dto.DeveloperCatalogBilling{Mode: "usage"}, true
	}
	if billing_setting.GetBillingMode(billingName) == billing_setting.BillingModeTieredExpr {
		expression, found := billing_setting.GetBillingExpr(billingName)
		if !found || strings.TrimSpace(expression) == "" {
			return pricing, billing, false
		}
		if _, err := billingexpr.CompileFromCache(expression); err != nil {
			return pricing, billing, false
		}
		pricing, billing = catalogExpressionPricing(expression, nil, multiplier)
		if pricing.PerImage != nil && path != "/v1/images/generations" && path != "/v1/images/edits" {
			return dto.DeveloperCatalogPricing{Variable: true}, dto.DeveloperCatalogBilling{Mode: "variable", Features: billing.Features}, true
		}
		return pricing, billing, true
	}
	if price, fixed := ratio_setting.GetModelPrice(billingName, false); fixed {
		if price < 0 || math.IsNaN(price) || math.IsInf(price, 0) {
			return pricing, billing, false
		}
		value := decimal.NewFromFloat(price).Mul(multiplier).String()
		billing.Mode = "request"
		if path == "/v1/images/generations" || path == "/v1/images/edits" {
			pricing.PerImage = &value
			billing.Features = []string{"image"}
			if len(ResolveLegacyBillingDetails(name, PricingValues{"ModelPrice": price}, nil).RequestRules) > 0 {
				pricing = dto.DeveloperCatalogPricing{Variable: true}
			}
		} else {
			pricing.PerRequest = &value
			billing.Features = []string{"request"}
		}
		return pricing, billing, true
	}
	ratio, configured, _ := ratio_setting.GetModelRatio(billingName)
	if !configured && !acceptUnset {
		return pricing, billing, false
	}
	base, err := legacyInputPricePerMillion(ratio)
	if err != nil {
		return pricing, billing, false
	}
	// These legacy branches derive or select rates using upstream-specific
	// runtime facts. Their base ratio alone is not a lossless public quote.
	if strings.Contains(strings.ToLower(billingName), "realtime") || channelType == constant.ChannelTypeOpenRouter && strings.Contains(strings.ToLower(billingName), "claude") {
		return dto.DeveloperCatalogPricing{Variable: true}, dto.DeveloperCatalogBilling{Mode: "variable"}, true
	}
	input := base.Mul(multiplier).String()
	completion := ratio_setting.GetCompletionRatio(billingName)
	if completion < 0 || math.IsNaN(completion) || math.IsInf(completion, 0) {
		return pricing, billing, false
	}
	output := base.Mul(decimal.NewFromFloat(completion)).Mul(multiplier).String()
	pricing.InputPer1MTokens, pricing.OutputPer1MTokens = &input, &output
	if cache, exists := ratio_setting.GetCacheRatio(billingName); exists && cache >= 0 && !math.IsNaN(cache) && !math.IsInf(cache, 0) {
		value := base.Mul(decimal.NewFromFloat(cache)).Mul(multiplier).String()
		pricing.CachedInputPer1MTokens = &value
	}
	billing.Features = []string{"input_token", "output_token"}
	if pricing.CachedInputPer1MTokens != nil {
		billing.Features = append(billing.Features, "cached_input")
	}
	effective := PricingValues{"ModelRatio": ratio, "CompletionRatio": completion}
	configuredPrices := PricingValues{}
	cache, _ := ratio_setting.GetCacheRatio(billingName)
	effective["CacheRatio"] = cache
	image, hasImage := ratio_setting.GetImageRatio(billingName)
	effective["ImageRatio"] = image
	createCache, hasCreateCache := ratio_setting.GetCreateCacheRatio(billingName)
	effective["CreateCacheRatio"] = createCache
	if hasCreateCache {
		configuredPrices["CreateCacheRatio"] = createCache
	}
	if ratio_setting.ContainsAudioRatio(billingName) {
		effective["AudioRatio"] = ratio_setting.GetAudioRatio(billingName)
	}
	if ratio_setting.ContainsAudioCompletionRatio(billingName) {
		effective["AudioCompletionRatio"] = ratio_setting.GetAudioCompletionRatio(billingName)
	}
	details := ResolveLegacyBillingDetails(billingName, effective, configuredPrices)
	if details.InvalidAudioPrice || details.ConflictingAudioPrices || details.AudioTextBranches {
		return dto.DeveloperCatalogPricing{Variable: true}, dto.DeveloperCatalogBilling{Mode: "variable"}, true
	}
	for _, component := range []struct {
		subject string
		price   *float64
	}{
		{"audio_input", details.AudioInputPrice},
		{"audio_output", details.AudioOutputPrice},
	} {
		if component.price != nil {
			if *component.price < 0 || math.IsNaN(*component.price) || math.IsInf(*component.price, 0) {
				return pricing, billing, false
			}
			pricing.Components = append(pricing.Components, dto.DeveloperCatalogPriceComponent{Subject: component.subject, Unit: "token_per_1m", Price: decimal.NewFromFloat(*component.price).Mul(multiplier).String()})
			billing.Features = append(billing.Features, component.subject)
		}
	}
	cacheWriteMode := ResolveCacheWriteMode(billingName, configuredPrices)
	for _, component := range []struct {
		subject string
		ratio   float64
		factor  decimal.Decimal
		enabled bool
	}{
		{"image_input", image, decimal.NewFromInt(1), hasImage},
		{"cached_input_write", createCache, decimal.NewFromInt(1), cacheWriteMode != CacheWriteNone},
		{"cached_input_write_1h", createCache, decimal.NewFromInt(6).Div(decimal.New(375, -2)), cacheWriteMode == CacheWriteClaudeTTL},
	} {
		if component.enabled {
			if component.ratio < 0 || math.IsNaN(component.ratio) || math.IsInf(component.ratio, 0) {
				return pricing, billing, false
			}
			pricing.Components = append(pricing.Components, dto.DeveloperCatalogPriceComponent{Subject: component.subject, Unit: "token_per_1m", Price: base.Mul(decimal.NewFromFloat(component.ratio)).Mul(component.factor).Mul(multiplier).String()})
			billing.Features = append(billing.Features, component.subject)
		}
	}
	slices.SortFunc(pricing.Components, func(a, b dto.DeveloperCatalogPriceComponent) int { return strings.Compare(a.Subject, b.Subject) })
	slices.Sort(billing.Features)
	return pricing, billing, true
}

type catalogLinearPrice struct {
	constant decimal.Decimal
	terms    map[string]decimal.Decimal
}

// catalogLinearExpression accepts only exact additive unit prices. It does not
// sample expressions or project optimized ASTs, which can discard zero prices.
func catalogLinearExpression(node ast.Node) (catalogLinearPrice, bool) {
	value := catalogLinearPrice{terms: make(map[string]decimal.Decimal)}
	switch node := node.(type) {
	case *ast.IntegerNode:
		value.constant = decimal.NewFromInt(int64(node.Value))
		return value, node.Value >= 0
	case *ast.FloatNode:
		if math.IsInf(node.Value, 0) || math.IsNaN(node.Value) || node.Value < 0 {
			return value, false
		}
		value.constant = decimal.NewFromFloat(node.Value)
		return value, true
	case *ast.IdentifierNode:
		if !slices.Contains([]string{"p", "c", "cr", "cc", "cc1h", "img", "img_cr", "ai", "ao", "img_o"}, node.Value) {
			return value, false
		}
		value.terms[node.Value] = decimal.NewFromInt(1)
		return value, true
	case *ast.CallNode:
		callee, ok := node.Callee.(*ast.IdentifierNode)
		if !ok {
			return value, false
		}
		if callee.Value == "tier" && len(node.Arguments) == 2 {
			if _, ok := node.Arguments[0].(*ast.StringNode); !ok {
				return value, false
			}
			return catalogLinearExpression(node.Arguments[1])
		}
		if (callee.Value == "u" || callee.Value == "fixed") && len(node.Arguments) == 1 {
			if callee.Value == "u" {
				key, ok := node.Arguments[0].(*ast.StringNode)
				if !ok {
					return value, false
				}
				value.terms["u:"+key.Value] = decimal.NewFromInt(1)
				return value, true
			}
			amount, ok := catalogLinearExpression(node.Arguments[0])
			if !ok || len(amount.terms) != 0 {
				return value, false
			}
			value.terms["request"] = amount.constant
			return value, true
		}
	case *ast.BinaryNode:
		left, ok := catalogLinearExpression(node.Left)
		if !ok {
			return value, false
		}
		right, ok := catalogLinearExpression(node.Right)
		if !ok {
			return value, false
		}
		switch node.Operator {
		case "+":
			left.constant = left.constant.Add(right.constant)
			for key, coefficient := range right.terms {
				left.terms[key] = left.terms[key].Add(coefficient)
			}
			return left, true
		case "*":
			if len(right.terms) != 0 {
				left, right = right, left
			}
			if len(right.terms) != 0 {
				return value, false
			}
			left.constant = left.constant.Mul(right.constant)
			for key, coefficient := range left.terms {
				left.terms[key] = coefficient.Mul(right.constant)
			}
			return left, true
		case "/":
			// Only decimal scale divisions have a finite, exact projection.
			// In particular canonical task token prices divide by one million.
			if len(right.terms) != 0 || !right.constant.Equal(decimal.NewFromInt(1_000_000)) {
				return value, false
			}
			left.constant = left.constant.Div(right.constant)
			for key, coefficient := range left.terms {
				left.terms[key] = coefficient.Div(right.constant)
			}
			return left, true
		}
	}
	return value, false
}

func catalogExpressionPricing(expression string, schema map[string]jsplugin.UsageFieldSchema, multiplier decimal.Decimal) (dto.DeveloperCatalogPricing, dto.DeveloperCatalogBilling) {
	variable := dto.DeveloperCatalogPricing{Variable: true}
	billing := dto.DeveloperCatalogBilling{Mode: "variable"}
	// Billing dimensions remain useful even when conditions or request rules
	// prevent a lossless flat-price projection. Only known units are public.
	if schema != nil {
		for key := range billingexpr.UsedUsageKeys(expression) {
			if field, ok := schema[key]; ok && field.Type == "number" {
				billing.Features = append(billing.Features, field.Unit)
			}
		}
	} else {
		features := map[string]string{"p": "input_token", "c": "output_token", "cr": "cached_input", "cc": "cached_input_write", "cc1h": "cached_input_write_1h", "img": "image_input", "img_cr": "cached_image_input", "ai": "audio_input", "ao": "audio_output", "img_o": "image_output", "image_count": "image"}
		for key := range billingexpr.UsedVars(expression) {
			if feature := features[key]; feature != "" {
				billing.Features = append(billing.Features, feature)
			}
		}
		if billingexpr.UsesFixedPricing(expression) {
			billing.Features = append(billing.Features, "request")
		}
	}
	slices.Sort(billing.Features)
	billing.Features = slices.Compact(billing.Features)
	version, body := billingexpr.ParseExprVersion(expression)
	if version != 1 {
		return variable, billing
	}
	tree, err := parser.Parse(body)
	if err != nil {
		return variable, billing
	}
	// Fixed per-image pricing is the sole supported product of two units.
	if node, ok := tree.Node.(*ast.BinaryNode); ok && node.Operator == "*" {
		if quantity, ok := node.Right.(*ast.IdentifierNode); ok && quantity.Value == "image_count" {
			price, valid := catalogLinearExpression(node.Left)
			if valid && len(price.terms) == 1 && price.constant.IsZero() {
				if fixed, ok := price.terms["request"]; ok {
					value := fixed.Mul(multiplier).String()
					return dto.DeveloperCatalogPricing{PerImage: &value}, dto.DeveloperCatalogBilling{Mode: "request", Features: []string{"image"}}
				}
			}
		}
	}
	linear, ok := catalogLinearExpression(tree.Node)
	if !ok {
		return variable, billing
	}
	if len(linear.terms) == 0 {
		if schema != nil {
			value := linear.constant.Mul(multiplier).String()
			return dto.DeveloperCatalogPricing{PerRequest: &value}, dto.DeveloperCatalogBilling{Mode: "request", Features: []string{"request"}}
		}
		return variable, billing
	}
	pricing := dto.DeveloperCatalogPricing{}
	if price, fixed := linear.terms["request"]; fixed {
		if len(linear.terms) != 1 || !linear.constant.IsZero() || schema != nil {
			return variable, billing
		}
		value := price.Mul(multiplier).String()
		return dto.DeveloperCatalogPricing{PerRequest: &value}, dto.DeveloperCatalogBilling{Mode: "request"}
	}
	if schema == nil && !linear.constant.IsZero() {
		return variable, billing
	}
	if schema == nil {
		billing.Mode = "token"
	} else {
		billing.Mode = "usage"
		if !linear.constant.IsZero() {
			value := linear.constant.Mul(multiplier).String()
			pricing.PerRequest = &value
		}
	}
	billing.Features = nil
	keys := make([]string, 0, len(linear.terms))
	for key := range linear.terms {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	for _, key := range keys {
		coefficient := linear.terms[key]
		value := coefficient.Mul(multiplier).String()
		if strings.HasPrefix(key, "u:") {
			if schema == nil {
				return variable, dto.DeveloperCatalogBilling{Mode: "variable"}
			}
			subject := strings.TrimPrefix(key, "u:")
			field, exists := schema[subject]
			if !exists || field.Type != "number" {
				return variable, dto.DeveloperCatalogBilling{Mode: "variable"}
			}
			switch field.Unit {
			case "second":
				if pricing.PerSecond == nil {
					pricing.PerSecond = &value
				} else {
					return variable, dto.DeveloperCatalogBilling{Mode: "variable"}
				}
			case "character":
				if pricing.PerCharacter == nil {
					pricing.PerCharacter = &value
				} else {
					return variable, dto.DeveloperCatalogBilling{Mode: "variable"}
				}
			case "count":
				if subject == "image_count" {
					pricing.PerImage = &value
				} else {
					pricing.Components = append(pricing.Components, dto.DeveloperCatalogPriceComponent{Subject: subject, Unit: "count", Price: value})
				}
			case "token":
				value = coefficient.Mul(decimal.NewFromInt(1_000_000)).Mul(multiplier).String()
				pricing.Components = append(pricing.Components, dto.DeveloperCatalogPriceComponent{Subject: subject, Unit: "token_per_1m", Price: value})
			case "credit":
				pricing.Components = append(pricing.Components, dto.DeveloperCatalogPriceComponent{Subject: subject, Unit: "credit", Price: value})
			default:
				return variable, dto.DeveloperCatalogBilling{Mode: "variable"}
			}
			billing.Features = append(billing.Features, field.Unit)
			continue
		}
		if schema != nil {
			return variable, dto.DeveloperCatalogBilling{Mode: "variable"}
		}
		switch key {
		case "p":
			pricing.InputPer1MTokens = &value
			billing.Features = append(billing.Features, "input_token")
		case "c":
			pricing.OutputPer1MTokens = &value
			billing.Features = append(billing.Features, "output_token")
		case "cr":
			pricing.CachedInputPer1MTokens = &value
			billing.Features = append(billing.Features, "cached_input")
		default:
			subjects := map[string]string{"cc": "cached_input_write", "cc1h": "cached_input_write_1h", "img": "image_input", "img_cr": "cached_image_input", "ai": "audio_input", "ao": "audio_output", "img_o": "image_output"}
			subject := subjects[key]
			if subject == "" {
				return variable, dto.DeveloperCatalogBilling{Mode: "variable"}
			}
			pricing.Components = append(pricing.Components, dto.DeveloperCatalogPriceComponent{Subject: subject, Unit: "token_per_1m", Price: value})
			billing.Features = append(billing.Features, subject)
		}
	}
	slices.Sort(billing.Features)
	billing.Features = slices.Compact(billing.Features)
	return pricing, billing
}
