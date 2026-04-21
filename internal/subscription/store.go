package subscription

import "time"

type SubscriptionList struct {
	Subscriptions []Subscription `toml:"subscription" json:"subscriptions"`
}

type Subscription struct {
	ID          string             `toml:"id" json:"id"`
	Name        string             `toml:"name" json:"name"`
	Type        string             `toml:"type" json:"type"`
	URL         string             `toml:"url,omitempty" json:"url,omitempty"`
	UserAgent   string             `toml:"user_agent,omitempty" json:"user_agent,omitempty"`
	Interval    string             `toml:"interval,omitempty" json:"interval,omitempty"`
	Enabled     bool               `toml:"enabled" json:"enabled"`
	LastUpdated time.Time          `toml:"last_updated,omitempty" json:"last_updated,omitempty"`
	NodeCount   int                `toml:"node_count,omitempty" json:"node_count,omitempty"`
	Filter      SubscriptionFilter `toml:"filter" json:"filter"`
}

type SubscriptionFilter struct {
	Include  []string `toml:"include,omitempty" json:"include,omitempty"`
	Exclude  []string `toml:"exclude,omitempty" json:"exclude,omitempty"`
	MaxNodes int      `toml:"max_nodes,omitempty" json:"max_nodes,omitempty"`
}

type ProxyNode struct {
	Name        string                 `json:"name"`
	Type        string                 `json:"type"`
	Server      string                 `json:"server"`
	Port        int                    `json:"port"`
	Extra       map[string]interface{} `json:"extra,omitempty"`
	SourceSubID string                 `json:"source_sub_id,omitempty"`
}
