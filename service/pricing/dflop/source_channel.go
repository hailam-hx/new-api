package dflop

import (
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

type SourceChannel struct {
	ID      int    `json:"id"`
	Name    string `json:"name"`
	Status  int    `json:"status"`
	BaseURL string `json:"base_url"`
}

func dflopBaseURL(raw string) bool {
	parsed, err := url.Parse(raw)
	return err == nil && parsed.Scheme == "https" && parsed.Host == "api.dflop.top" && (parsed.Path == "" || parsed.Path == "/") && parsed.RawQuery == "" && parsed.Fragment == "" && parsed.User == nil
}

func LoadSourceChannel(id int) (SourceChannel, string, error) {
	if id <= 0 {
		return SourceChannel{}, "", errors.New("SOURCE_CHANNEL_UNAVAILABLE: select a DFLOP source channel")
	}
	channel, err := model.GetChannelById(id, true)
	if err != nil {
		return SourceChannel{}, "", fmt.Errorf("SOURCE_CHANNEL_UNAVAILABLE: %w", err)
	}
	info := SourceChannel{ID: channel.Id, Name: channel.Name, Status: channel.Status, BaseURL: channel.GetBaseURL()}
	if channel.Status != common.ChannelStatusEnabled || !dflopBaseURL(info.BaseURL) {
		return info, "", errors.New("SOURCE_CHANNEL_UNAVAILABLE: channel must be enabled and point to api.dflop.top")
	}
	if channel.ChannelInfo.IsMultiKey || len(channel.GetKeys()) != 1 || channel.GetKeys()[0] != channel.Key {
		return info, "", errors.New("MULTI_KEY_SOURCE_UNSUPPORTED: select a single-key DFLOP channel")
	}
	if strings.TrimSpace(channel.Key) == "" {
		return info, "", errors.New("SOURCE_CHANNEL_UNAVAILABLE: selected channel has no credential")
	}
	return info, channel.Key, nil
}

func SourceChannelCandidates() ([]SourceChannel, error) {
	var candidates []SourceChannel
	for offset := 0; ; offset += 100 {
		channels, err := model.GetAllChannels(offset, 100, false, true)
		if err != nil {
			return nil, err
		}
		for _, channel := range channels {
			base := channel.GetBaseURL()
			if dflopBaseURL(base) {
				candidates = append(candidates, SourceChannel{ID: channel.Id, Name: channel.Name, Status: channel.Status, BaseURL: base})
			}
		}
		if len(channels) < 100 {
			return candidates, nil
		}
	}
}

// ApprovedCatalogOrigin shares the reviewed source-channel origin policy.
func ApprovedCatalogOrigin(raw string) bool { return dflopBaseURL(raw) }

// EndpointBinding is the canonical interpretation of an authenticated endpoint
// type. Dedicated media endpoints take precedence over incidental chat flags.
type EndpointBinding struct {
	Protocol string `json:"protocol"`
	Path     string `json:"path"`
}

func CatalogEndpoint(item Item) (EndpointBinding, error) {
	if !item.Callable {
		return EndpointBinding{}, errors.New("SOURCE_MODEL_NOT_CALLABLE")
	}
	switch item.EndpointType {
	case "images_generations":
		return EndpointBinding{"openai_image", "/v1/images/generations"}, nil
	case "videos_generations":
		return EndpointBinding{"openai_video", "/v1/videos/generations"}, nil
	case "tts_synthesize":
		return EndpointBinding{"openai_audio_speech", "/v1/audio/speech"}, nil
	case "music_generations":
		return EndpointBinding{"native", "/dflop/v1/music/generations"}, nil
	case "voice_clone":
		return EndpointBinding{"native", "/dflop/v1/audio/voices"}, nil
	case "avatar_create":
		return EndpointBinding{"native", "/dflop/v1/videos/avatars"}, nil
	case "", "chat_completions":
		if slices.Contains(item.Protocols, "openai_chat") {
			return EndpointBinding{"openai_chat", "/v1/chat/completions"}, nil
		}
	}
	return EndpointBinding{}, errors.New("PROTOCOL_MISMATCH: authenticated endpoint has no host binding")
}

// CatalogEndpointForPath validates an explicit protocol without letting a
// dedicated media model fall through to chat.
func CatalogEndpointForPath(item Item, path string) (EndpointBinding, error) {
	binding, err := CatalogEndpoint(item)
	if err != nil {
		return binding, err
	}
	if binding.Path == path {
		return binding, nil
	}
	if item.EndpointType == "" || item.EndpointType == "chat_completions" {
		for protocol, endpoint := range map[string]string{"anthropic_messages": "/v1/messages", "openai_responses": "/v1/responses", "gemini_native": "/v1beta/models/{model}:generateContent"} {
			if path == endpoint && slices.Contains(item.Protocols, protocol) {
				return EndpointBinding{protocol, endpoint}, nil
			}
		}
	}
	return EndpointBinding{}, errors.New("PROTOCOL_MISMATCH: authenticated DFLOP contract requires " + binding.Path)
}

// AuthenticatedSourceCatalog reuses the existing validated, credential-scoped
// last-good snapshot. It never fetches upstream or persists anything. A cold
// or unavailable source requires an explicit fresh preview/connectivity GET.
func AuthenticatedSourceCatalog(channelID int, key string) ([]byte, error) {
	if channelID <= 0 || strings.TrimSpace(key) == "" || model.DB == nil {
		return nil, errors.New("SOURCE_CATALOG_UNAVAILABLE")
	}
	body, _, err := lastGoodSource(channelID, sourceDigest([]byte(key))[:16])
	if err != nil || len(body) == 0 {
		return nil, errors.New("SOURCE_CATALOG_UNAVAILABLE: refresh authenticated catalog preview")
	}
	return body, nil
}

func AuthenticatedCatalogModel(body []byte, name string) (Item, error) {
	var catalog struct {
		Schema   string            `json:"schema_version"`
		Currency string            `json:"currency"`
		Aliases  map[string]string `json:"aliases"`
		Models   []struct {
			ID      string `json:"id"`
			Pricing struct {
				Callable     bool     `json:"callable"`
				EndpointType string   `json:"endpoint_type"`
				Protocols    []string `json:"supported_protocols"`
			} `json:"pricing"`
		} `json:"models"`
	}
	if common.Unmarshal(body, &catalog) != nil || catalog.Schema != "1.0" || catalog.Currency != "points" || catalog.Aliases == nil || catalog.Models == nil {
		return Item{}, errors.New("SOURCE_CATALOG_UNAVAILABLE: invalid authenticated contract")
	}
	byID := make(map[string]Item, len(catalog.Models))
	for _, m := range catalog.Models {
		byID[m.ID] = Item{ModelID: m.ID, CanonicalID: m.ID, Callable: m.Pricing.Callable, EndpointType: m.Pricing.EndpointType, Protocols: m.Pricing.Protocols}
	}
	seen := map[string]bool{}
	for catalog.Aliases[name] != "" && !seen[name] {
		if _, ok := byID[name]; ok {
			break
		}
		seen[name] = true
		name = catalog.Aliases[name]
	}
	item, ok := byID[name]
	if !ok {
		return Item{}, errors.New("SOURCE_MODEL_NOT_IN_AUTHENTICATED_CATALOG")
	}
	return item, nil
}
