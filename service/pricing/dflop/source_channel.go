package dflop

import (
	"errors"
	"fmt"
	"net/url"
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
