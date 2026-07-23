package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type DiscordChannelSpec struct {
	// Discord server ID
	ServerID string `json:"serverId"`

	// Channel name
	ChannelName string `json:"channelName"`

	// Discord bot token (stored as secret reference)
	BotTokenSecretRef string `json:"botTokenSecretRef,omitempty"`
}

type DiscordChannelStatus struct {
	// Status: "Configured", "Failed"
	Status string `json:"status,omitempty"`

	// Discord channel ID
	ChannelID string `json:"channelId,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

type DiscordChannel struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DiscordChannelSpec   `json:"spec,omitempty"`
	Status DiscordChannelStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

type DiscordChannelList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DiscordChannel `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DiscordChannel{}, &DiscordChannelList{})
}
