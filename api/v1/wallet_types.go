package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type WalletSpec struct {
	// Blockchain network. EVM networks (ethereum, polygon) share the same
	// secp256k1/keccak address format; bitcoin is not supported for generation yet.
	// +kubebuilder:validation:Enum=ethereum;polygon;bitcoin
	// +kubebuilder:default=ethereum
	Network string `json:"network,omitempty"`
}

type WalletStatus struct {
	// Phase: "Active" (keypair generated) or "Unsupported" (network).
	Phase string `json:"phase,omitempty"`

	// Generated public wallet address.
	Address string `json:"address,omitempty"`

	// Name of the Secret holding the private key (`<name>-wallet`).
	SecretRef string `json:"secretRef,omitempty"`

	// Balance (placeholder until an on-chain lookup is wired in).
	Balance string `json:"balance,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="Network",type=string,JSONPath=`.spec.network`
//+kubebuilder:printcolumn:name="Address",type=string,JSONPath=`.status.address`
//+kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`

type Wallet struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   WalletSpec   `json:"spec,omitempty"`
	Status WalletStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

type WalletList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Wallet `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Wallet{}, &WalletList{})
}
